package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AWSRegion      string
	AlbumsTable    string
	PhotosTable    string
	AlbumSeqTable  string
	S3Bucket       string
	SQSQueueURL    string
	Port           string
	PresignTTL     time.Duration
	WorkerParallel  int
	UploaderCount   int
}

func MustLoad() Config {
	cfg := Config{
		AWSRegion:      getenv("AWS_REGION", "us-west-2"),
		AlbumsTable:    getenv("ALBUMS_TABLE", "albums"),
		PhotosTable:    getenv("PHOTOS_TABLE", "photos"),
		AlbumSeqTable:  getenv("ALBUM_SEQ_TABLE", "album_seq"),
		S3Bucket:       getenv("S3_BUCKET", ""),
		SQSQueueURL:    getenv("SQS_QUEUE_URL", ""),
		Port:           getenv("PORT", "8080"),
		PresignTTL:     parseMinutes(getenv("PRESIGN_TTL_MIN", "120")),
		WorkerParallel:  parseInt(getenv("WORKER_PARALLEL", "5"), 5),
		UploaderCount:   parseInt(getenv("UPLOADER_COUNT", "11"), 11),
	}

	var missing []string
	if cfg.S3Bucket == "" {
		missing = append(missing, "S3_BUCKET")
	}
	if cfg.SQSQueueURL == "" {
		missing = append(missing, "SQS_QUEUE_URL")
	}
	if len(missing) > 0 {
		panic(fmt.Sprintf("missing required env vars: %s", strings.Join(missing, ", ")))
	}
	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseMinutes(s string) time.Duration {
	mins, err := strconv.Atoi(s)
	if err != nil || mins <= 0 {
		mins = 120
	}
	return time.Duration(mins) * time.Minute
}

func parseInt(s string, fallback int) int {
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
