package s3util

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Client struct {
	s3       *s3.Client
	uploader *manager.Uploader
	presign  *s3.PresignClient
	bucket   string
}

func New(s3c *s3.Client, bucket string) *Client {
	uploader := manager.NewUploader(s3c, func(u *manager.Uploader) {
		u.PartSize         = 5 * 1024 * 1024 // 5 MB per part — triggers multipart for smaller files
		u.Concurrency      = 5              // 5 parallel parts per file
		u.LeavePartsOnError = false
	})
	return &Client{
		s3:       s3c,
		uploader: uploader,
		presign:  s3.NewPresignClient(s3c),
		bucket:   bucket,
	}
}

// PutObject uses the S3 transfer manager, which automatically switches to
// multipart upload for files larger than PartSize (10 MB).
// size must be the exact byte length of body; it is sent as Content-Length so
// the SDK avoids chunked transfer encoding and skips a seek to determine size.
func (c *Client) PutObject(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := c.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:        &c.bucket,
		Key:           &key,
		Body:          body,
		ContentType:   &contentType,
		ContentLength: &size,
	})
	return err
}

func (c *Client) CopyObject(ctx context.Context, srcKey, dstKey string) error {
	copySource := fmt.Sprintf("%s/%s", c.bucket, srcKey)
	_, err := c.s3.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:            &c.bucket,
		Key:               &dstKey,
		CopySource:        &copySource,
		MetadataDirective: types.MetadataDirectiveCopy,
	})
	return err
}

func (c *Client) DeleteObject(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &c.bucket, Key: &key})
	return err
}

func (c *Client) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	out, err := c.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &c.bucket, Key: &key}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}
