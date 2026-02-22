// Package main provides a one-shot setup CLI that configures MinIO lifecycle
// policies for the Argus AI platform. It sets ILM (Information Lifecycle
// Management) expiration rules to auto-delete temporary objects.
//
// Usage:
//
//	go run ./cmd/setup
//
// Environment variables (same as the event-collector):
//
//	EVENT_COLLECTOR_MINIO_ENDPOINT   — MinIO host:port (default: localhost:9002)
//	EVENT_COLLECTOR_MINIO_ACCESS_KEY — Access key
//	EVENT_COLLECTOR_MINIO_SECRET_KEY — Secret key
//	EVENT_COLLECTOR_MINIO_USE_SSL    — TLS (default: false)
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

const (
	defaultEndpoint  = "localhost:9002"
	defaultAccessKey = "argus-minio-admin"
	defaultSecretKey = "argus-minio-secret-change-me-32ch"

	// exportsBucket holds temporary evidence archives (ZIP exports).
	// These are ephemeral — a 30-day lifecycle expiration auto-purges them.
	exportsBucket = "argus-exports"

	// expirationDays is the lifecycle expiration for the exports bucket.
	expirationDays = 30
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	endpoint := env("EVENT_COLLECTOR_MINIO_ENDPOINT", defaultEndpoint)
	accessKey := env("EVENT_COLLECTOR_MINIO_ACCESS_KEY", defaultAccessKey)
	secretKey := env("EVENT_COLLECTOR_MINIO_SECRET_KEY", defaultSecretKey)
	useSSL := strings.EqualFold(env("EVENT_COLLECTOR_MINIO_USE_SSL", "false"), "true")

	log.Printf("connecting to MinIO at %s (ssl=%v)", endpoint, useSSL)

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Fatalf("failed to create MinIO client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify bucket exists.
	exists, err := client.BucketExists(ctx, exportsBucket)
	if err != nil {
		log.Fatalf("failed to check bucket %q: %v", exportsBucket, err)
	}
	if !exists {
		log.Printf("bucket %q does not exist — creating...", exportsBucket)
		if err := client.MakeBucket(ctx, exportsBucket, minio.MakeBucketOptions{}); err != nil {
			log.Fatalf("failed to create bucket %q: %v", exportsBucket, err)
		}
		log.Printf("bucket %q created", exportsBucket)
	}

	// Set 30-day expiration lifecycle on exports bucket.
	lcConfig := lifecycle.Configuration{
		Rules: []lifecycle.Rule{
			{
				ID:     "auto-expire-exports-30d",
				Status: "Enabled",
				Expiration: lifecycle.Expiration{
					Days: lifecycle.ExpirationDays(expirationDays),
				},
			},
		},
	}

	if err := client.SetBucketLifecycle(ctx, exportsBucket, &lcConfig); err != nil {
		log.Fatalf("failed to set lifecycle on %q: %v", exportsBucket, err)
	}

	fmt.Printf("lifecycle policy set: %s → %d-day expiration\n", exportsBucket, expirationDays)
	fmt.Println("setup complete")
}
