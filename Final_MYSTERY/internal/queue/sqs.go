package queue

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type PhotoJob struct {
	PhotoID    string `json:"photo_id"`
	AlbumID    string `json:"album_id"`
	StagingKey string `json:"staging_key"`
}

type Client struct {
	sqs      *sqs.Client
	queueURL string
}

func New(s *sqs.Client, queueURL string) *Client {
	return &Client{sqs: s, queueURL: queueURL}
}

func (c *Client) SendPhotoJob(ctx context.Context, job PhotoJob) error {
	b, err := json.Marshal(job)
	if err != nil {
		return err
	}
	_, err = c.sqs.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: &c.queueURL, MessageBody: strPtr(string(b))})
	return err
}

func strPtr(s string) *string { return &s }
