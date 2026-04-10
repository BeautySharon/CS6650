package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"album-store/internal/queue"
	"album-store/internal/s3util"
	"album-store/internal/store"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
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
	n := max(1, p.Parallel)
	log.Printf("worker started, queue=%s parallel=%d", p.QueueURL, n)

	// Persistent worker pool. Closing jobs signals workers to stop.
	jobs := make(chan typesMessage, n*10)
	defer close(jobs)

	// toDelete collects receipt handles; runBatchDeleter flushes them in batches of 10.
	toDelete := make(chan *string, n)
	go p.runBatchDeleter(ctx, toDelete)

	for i := 0; i < n; i++ {
		go func() {
			for m := range jobs {
				body := awsString(m.Body)
				log.Printf("processing raw message: %s", body)
				if err := p.handleOne(ctx, body); err != nil {
					log.Printf("handleOne failed: %v", err)
					continue // leave in queue; visibility timeout re-delivers
				}
				select {
				case toDelete <- m.ReceiptHandle:
				case <-ctx.Done():
				}
			}
		}()
	}

	backoff := time.Second
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
			VisibilityTimeout:   120,
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("receive error (retrying in %s): %v", backoff, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second

		if len(out.Messages) == 0 {
			continue
		}
		log.Printf("received %d message(s)", len(out.Messages))

		for _, msg := range out.Messages {
			select {
			case jobs <- typesMessage{Body: msg.Body, ReceiptHandle: msg.ReceiptHandle}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
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

// batchIDs provides the fixed entry IDs required by DeleteMessageBatch.
var batchIDs = [10]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}

// runBatchDeleter accumulates receipt handles and flushes them via
// DeleteMessageBatch (up to 10 per call) every 500 ms or when the batch is full.
func (p *Processor) runBatchDeleter(ctx context.Context, toDelete <-chan *string) {
	var handles []*string
	flush := func() {
		if len(handles) == 0 {
			return
		}
		entries := make([]sqstypes.DeleteMessageBatchRequestEntry, len(handles))
		for i, h := range handles {
			entries[i] = sqstypes.DeleteMessageBatchRequestEntry{
				Id:            &batchIDs[i],
				ReceiptHandle: h,
			}
		}
		if _, err := p.SQS.DeleteMessageBatch(ctx, &sqs.DeleteMessageBatchInput{
			QueueUrl: &p.QueueURL,
			Entries:  entries,
		}); err != nil {
			log.Printf("batch delete failed: %v", err)
		}
		handles = handles[:0]
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case h, ok := <-toDelete:
			if !ok {
				flush()
				return
			}
			handles = append(handles, h)
			if len(handles) == 10 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}

func (p *Processor) handleOne(ctx context.Context, body string) error {
	var job queue.PhotoJob
	if err := json.Unmarshal([]byte(body), &job); err != nil {
		return fmt.Errorf("unmarshal job failed: %w", err)
	}

	log.Printf("job parsed: album_id=%s photo_id=%s key=%s", job.AlbumID, job.PhotoID, job.StagingKey)

	// File was uploaded directly to its final location; no copy needed.
	finalKey := job.StagingKey

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

	log.Printf("job completed successfully: photo_id=%s", job.PhotoID)
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
