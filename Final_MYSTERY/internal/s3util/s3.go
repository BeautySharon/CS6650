package s3util

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Client struct {
	s3      *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func New(s3c *s3.Client, bucket string) *Client {
	return &Client{s3: s3c, presign: s3.NewPresignClient(s3c), bucket: bucket}
}

func (c *Client) PutObject(ctx context.Context, key string, body io.Reader, contentType string) error {
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{Bucket: &c.bucket, Key: &key, Body: body, ContentType: &contentType})
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
