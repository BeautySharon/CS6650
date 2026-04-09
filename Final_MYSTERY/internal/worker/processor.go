package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"album-store/internal/queue"
	"album-store/internal/s3util"
	"album-store/internal/store"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type Processor struct {
	SQS        *sqs.Client
	QueueURL   string
	Store      *store.DynamoStore
	S3         *s3util.Client
	PresignTTL time.Duration
	Parallel   int
}

func (p *Processor) Run(ctx context.Context) error {
	log.Printf("worker started, queue=%s parallel=%d", p.QueueURL, max(1, p.Parallel))

	for {
		select {
		case <-ctx.Done():
			log.Printf("worker stopping: %v", ctx.Err())
			return ctx.Err()
		default:
		}

		out, err := p.SQS.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &p.QueueURL,
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20,
			VisibilityTimeout:   60,
		})
		if err != nil {
			log.Printf("receive message error: %v", err)
			return err
		}

		if len(out.Messages) == 0 {
			log.Printf("no messages received")
			continue
		}

		log.Printf("received %d message(s)", len(out.Messages))

		sem := make(chan struct{}, max(1, p.Parallel))
		var wg sync.WaitGroup

		for _, msg := range out.Messages {
			wg.Add(1)
			sem <- struct{}{}

			go func(m typesMessage) {
				defer wg.Done()
				defer func() { <-sem }()

				body := awsString(m.Body)
				log.Printf("processing raw message: %s", body)

				if err := p.handleOne(ctx, body); err != nil {
					log.Printf("handleOne failed: %v", err)
					return
				}

				_, err := p.SQS.DeleteMessage(ctx, &sqs.DeleteMessageInput{
					QueueUrl:      &p.QueueURL,
					ReceiptHandle: m.ReceiptHandle,
				})
				if err != nil {
					log.Printf("delete message failed: %v", err)
				} else {
					log.Printf("message deleted successfully")
				}
			}(typesMessage{
				Body:          msg.Body,
				ReceiptHandle: msg.ReceiptHandle,
			})
		}

		wg.Wait()
	}
}

type typesMessage struct {
	Body          *string
	ReceiptHandle *string
}

func awsString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (p *Processor) handleOne(ctx context.Context, body string) error {
	var job queue.PhotoJob
	if err := json.Unmarshal([]byte(body), &job); err != nil {
		return fmt.Errorf("unmarshal job failed: %w", err)
	}

	log.Printf("job parsed: album_id=%s photo_id=%s staging_key=%s", job.AlbumID, job.PhotoID, job.StagingKey)

	finalKey := fmt.Sprintf("albums/%s/%s", job.AlbumID, job.PhotoID)

	log.Printf("copy object: %s -> %s", job.StagingKey, finalKey)
	if err := p.S3.CopyObject(ctx, job.StagingKey, finalKey); err != nil {
		_ = p.Store.MarkPhotoFailed(ctx, job.PhotoID)
		return fmt.Errorf("copy object failed: %w", err)
	}

	log.Printf("presign get url for key=%s", finalKey)
	url, err := p.S3.PresignGet(ctx, finalKey, p.PresignTTL)
	if err != nil {
		_ = p.Store.MarkPhotoFailed(ctx, job.PhotoID)
		return fmt.Errorf("presign failed: %w", err)
	}

	log.Printf("mark photo completed: photo_id=%s", job.PhotoID)
	if err := p.Store.MarkPhotoCompleted(ctx, job.PhotoID, finalKey, url); err != nil {
		return fmt.Errorf("mark photo completed failed: %w", err)
	}

	log.Printf("delete staging object: %s", job.StagingKey)
	if err := p.S3.DeleteObject(ctx, job.StagingKey); err != nil {
		log.Printf("warning: failed to delete staging object %s: %v", job.StagingKey, err)
	}

	log.Printf("job completed successfully: photo_id=%s", job.PhotoID)
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}