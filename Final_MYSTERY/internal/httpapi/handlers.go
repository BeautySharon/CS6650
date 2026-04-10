package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"album-store/internal/model"
	"album-store/internal/s3util"
	"album-store/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type App struct {
	Store      *store.DynamoStore
	S3         *s3util.Client
	PresignTTL time.Duration
	UploadJobs chan UploadTask
	albumCache sync.Map // album_id -> struct{}{}; albums are never deleted so cache is permanent
}

// UploadTask carries everything a pool worker needs to complete an upload.
// For small files (≤32 MB), data holds the bytes in memory and tmpPath is empty.
// For large disk-backed files, tmpPath is set and the worker must os.Remove it.
type UploadTask struct {
	data        []byte // non-nil for in-memory files; avoids disk round-trip
	tmpPath     string // non-empty for disk-backed files; worker must remove
	size        int64
	contentType string
	key         string
	photoID     string
}

// StartUploadWorkers launches n persistent goroutines that drain UploadJobs.
// Call once at process startup before serving requests.
func (a *App) StartUploadWorkers(n int) {
	for i := 0; i < n; i++ {
		go func() {
			for t := range a.UploadJobs {
				a.processUpload(t)
			}
		}()
	}
}

// processUpload performs the three post-upload steps for one photo.
func (a *App) processUpload(t UploadTask) {
	ctx := context.Background()

	log.Printf("upload start photo=%s key=%s size=%d", t.photoID, t.key, t.size)

	var body io.Reader
	if t.data != nil {
		// Small file: already in memory — no disk I/O needed.
		body = bytes.NewReader(t.data)
	} else {
		// Large file: disk-backed temp file — must clean up after use.
		defer os.Remove(t.tmpPath)
		f, err := os.Open(t.tmpPath)
		if err != nil {
			log.Printf("upload failed photo=%s: %v", t.photoID, err)
			_ = a.Store.MarkPhotoFailed(ctx, t.photoID)
			return
		}
		defer f.Close()
		body = f
	}

	err := a.S3.PutObject(ctx, t.key, body, t.size, t.contentType)
	if err != nil {
		log.Printf("upload failed photo=%s: %v", t.photoID, err)
		_ = a.Store.MarkPhotoFailed(ctx, t.photoID)
		return
	}
	log.Printf("upload done photo=%s key=%s", t.photoID, t.key)

	url, err := a.S3.PresignGet(ctx, t.key, a.PresignTTL)
	if err != nil {
		log.Printf("presign failed photo=%s: %v", t.photoID, err)
		_ = a.Store.MarkPhotoFailed(ctx, t.photoID)
		return
	}
	log.Printf("presign done photo=%s", t.photoID)

	if err := a.Store.MarkPhotoCompleted(ctx, t.photoID, t.key, url); err != nil {
		log.Printf("mark completed failed photo=%s: %v", t.photoID, err)
		return
	}
	log.Printf("mark completed done photo=%s", t.photoID)
}

func NewRouter(app *App) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.MaxMultipartMemory = 32 << 20 // 32 MB before spilling to disk
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
	a.albumCache.Store(in.AlbumID, struct{}{})
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
	// Extract context once before any goroutines — Gin context must not be
	// accessed concurrently.
	ctx := c.Request.Context()

	if _, cached := a.albumCache.Load(albumID); !cached {
		if _, err := a.Store.GetAlbum(ctx, albumID); err == store.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		a.albumCache.Store(albumID, struct{}{})
	}

	fileHeader, err := c.FormFile("photo")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing photo"})
		return
	}

	photoID := uuid.NewString()
	stagingKey := fmt.Sprintf("albums/%s/%s%s", albumID, photoID, ext(fileHeader))

	seq, err := a.Store.NextSeq(ctx, albumID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	// Take ownership of the file before returning — O(1) rename for large files,
	// in-memory bytes for small ones. The async worker must not use fileHeader
	// after this point; Gin may clean up the original multipart temp file once
	// the handler returns.
	data, tmpPath, size, ct, err := stealFile(fileHeader)
	if err != nil {
		log.Printf("temp copy failed photo=%s: %v", photoID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	p := model.Photo{PhotoID: photoID, AlbumID: albumID, Seq: seq, Status: "processing", StagingKey: stagingKey}
	if err := a.Store.PutPhoto(ctx, p); err != nil {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"photo_id": photoID, "seq": seq, "status": "processing"})

	a.UploadJobs <- UploadTask{data: data, tmpPath: tmpPath, size: size, contentType: ct, key: stagingKey, photoID: photoID}
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
		// Prefer a fresh presigned URL; fall back to the stored URL if generation fails.
		if p.FinalKey != "" {
			if freshURL, err := a.S3.PresignGet(c.Request.Context(), p.FinalKey, a.PresignTTL); err == nil {
				resp["url"] = freshURL
			} else {
				resp["url"] = p.URL
			}
		} else {
			resp["url"] = p.URL
		}
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

// stealFile takes ownership of the multipart file before the handler returns,
// preventing Gin's cleanup from racing with the upload worker.
// For disk-backed files (>32 MB) it renames the existing temp file — one O(1)
// syscall, no data copied. For in-memory files it reads bytes into a []byte
// slice — no disk touch at all. Returns (data, tmpPath, size, contentType, err):
// exactly one of data or tmpPath will be non-zero.
func stealFile(fh *multipart.FileHeader) (data []byte, tmpPath string, size int64, contentType string, err error) {
	src, err := fh.Open()
	if err != nil {
		return nil, "", 0, "", err
	}
	defer src.Close()

	ct := fh.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}

	// Large files: Gin spilled to a real *os.File temp. Rename it — O(1) syscall.
	if osFile, ok := src.(*os.File); ok {
		dst := osFile.Name() + ".as"
		if rerr := os.Rename(osFile.Name(), dst); rerr == nil {
			return nil, dst, fh.Size, ct, nil
		}
		// Cross-device rename failed; fall through to in-memory copy.
	}

	// Small files (≤32 MB): source is in RAM. Read into a byte slice — no disk I/O.
	buf, err := io.ReadAll(src)
	if err != nil {
		return nil, "", 0, "", err
	}
	return buf, "", int64(len(buf)), ct, nil
}

func ext(fh *multipart.FileHeader) string {
	e := filepath.Ext(fh.Filename)
	if e == "" {
		return ".bin"
	}
	return e
}
