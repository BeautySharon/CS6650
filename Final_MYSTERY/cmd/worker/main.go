package main

import (
	"context"
	"log"

	"album-store/internal/config"
	"album-store/internal/s3util"
	"album-store/internal/store"
	workerpkg "album-store/internal/worker"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

func main() {
	cfg := config.MustLoad()
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		log.Fatal(err)
	}
	p := &workerpkg.Processor{
		SQS:        sqs.NewFromConfig(awsCfg),
		QueueURL:   cfg.SQSQueueURL,
		Store:      store.NewDynamoStore(dynamodb.NewFromConfig(awsCfg), cfg.AlbumsTable, cfg.PhotosTable, cfg.AlbumSeqTable),
		S3:         s3util.New(s3.NewFromConfig(awsCfg), cfg.S3Bucket),
		PresignTTL: cfg.PresignTTL,
		Parallel:   cfg.WorkerParallel,
	}
	log.Printf("worker started, queue=%s", cfg.SQSQueueURL)
	log.Fatal(p.Run(context.Background()))
}
