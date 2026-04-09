package httpapi

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"time"

	"album-store/internal/model"
	"album-store/internal/queue"
	"album-store/internal/s3util"
	"album-store/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type App struct {
	Store      *store.DynamoStore
	S3         *s3util.Client
	Queue      *queue.Client
	PresignTTL time.Duration
}

func NewRouter(app *App) *gin.Engine {
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.PUT("/albums/:album_id", app.PutAlbum)
	r.GET("/albums/:album_id", app.GetAlbum)
	r.GET("/albums", app.ListAlbums)
	r.POST("/albums/:album_id/photos", app.UploadPhoto)
	r.GET("/albums/:album_id/photos/:photo_id", app.GetPhoto)
	r.DELETE("/albums/:album_id/photos/:photo_id", app.DeletePhoto)
	return r
}

func (a *App) PutAlbum(c *gin.Context) {
	var in model.Album
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	albumID := c.Param("album_id")
	if in.AlbumID == "" {
		in.AlbumID = albumID
	}
	if in.AlbumID != albumID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "album_id mismatch"})
		return
	}
	if err := a.Store.PutAlbum(c.Request.Context(), in); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	c.JSON(http.StatusOK, in)
}

func (a *App) GetAlbum(c *gin.Context) {
	album, err := a.Store.GetAlbum(c.Request.Context(), c.Param("album_id"))
	if err == store.ErrNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	c.JSON(http.StatusOK, album)
}

func (a *App) ListAlbums(c *gin.Context) {
	albums, err := a.Store.ListAlbums(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"albums": albums})
}

func (a *App) UploadPhoto(c *gin.Context) {
	albumID := c.Param("album_id")
	if _, err := a.Store.GetAlbum(c.Request.Context(), albumID); err == store.ErrNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	fileHeader, err := c.FormFile("photo")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing photo"})
		return
	}

	photoID := uuid.NewString()
	seq, err := a.Store.NextSeq(c.Request.Context(), albumID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	stagingKey := fmt.Sprintf("staging/%s/%s%s", albumID, photoID, ext(fileHeader))
	if err := uploadMultipartFile(c, a.S3, fileHeader, stagingKey); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "upload failed"})
		return
	}

	p := model.Photo{PhotoID: photoID, AlbumID: albumID, Seq: seq, Status: "processing", StagingKey: stagingKey}
	if err := a.Store.PutPhoto(c.Request.Context(), p); err != nil {
		_ = a.S3.DeleteObject(c.Request.Context(), stagingKey)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	if err := a.Queue.SendPhotoJob(c.Request.Context(), queue.PhotoJob{PhotoID: photoID, AlbumID: albumID, StagingKey: stagingKey}); err != nil {
		_ = a.Store.DeletePhoto(c.Request.Context(), photoID)
		_ = a.S3.DeleteObject(c.Request.Context(), stagingKey)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "queue failed"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"photo_id": photoID, "seq": seq, "status": "processing"})
}

func (a *App) GetPhoto(c *gin.Context) {
	albumID := c.Param("album_id")
	photoID := c.Param("photo_id")
	p, err := a.Store.GetPhoto(c.Request.Context(), photoID)
	if err == store.ErrNotFound || (err == nil && p.AlbumID != albumID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	resp := gin.H{"photo_id": p.PhotoID, "album_id": p.AlbumID, "seq": p.Seq, "status": p.Status}
	if p.Status == "completed" {
		resp["url"] = p.URL
	}
	c.JSON(http.StatusOK, resp)
}

func (a *App) DeletePhoto(c *gin.Context) {
	albumID := c.Param("album_id")
	photoID := c.Param("photo_id")
	p, err := a.Store.GetPhoto(c.Request.Context(), photoID)
	if err == store.ErrNotFound || (err == nil && p.AlbumID != albumID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	_ = a.S3.DeleteObject(c.Request.Context(), p.StagingKey)
	_ = a.S3.DeleteObject(c.Request.Context(), p.FinalKey)
	if err := a.Store.DeletePhoto(c.Request.Context(), photoID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	c.Status(http.StatusNoContent)
}

func uploadMultipartFile(c *gin.Context, s3c *s3util.Client, fh *multipart.FileHeader, key string) error {
	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()
	contentType := fh.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return s3c.PutObject(c.Request.Context(), key, f, contentType)
}

func ext(fh *multipart.FileHeader) string {
	e := filepath.Ext(fh.Filename)
	if e == "" {
		return ".bin"
	}
	return e
}
