package main

import (
	"context"
	"log"

	"album-store/internal/config"
	"album-store/internal/httpapi"
	"album-store/internal/queue"
	"album-store/internal/s3util"
	"album-store/internal/store"

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
	app := &httpapi.App{
		Store:      store.NewDynamoStore(dynamodb.NewFromConfig(awsCfg), cfg.AlbumsTable, cfg.PhotosTable, cfg.AlbumSeqTable),
		S3:         s3util.New(s3.NewFromConfig(awsCfg), cfg.S3Bucket),
		Queue:      queue.New(sqs.NewFromConfig(awsCfg), cfg.SQSQueueURL),
		PresignTTL: cfg.PresignTTL,
	}
	r := httpapi.NewRouter(app)
	log.Printf("API listening on :%s", cfg.Port)
	log.Fatal(r.Run(":" + cfg.Port))
}
