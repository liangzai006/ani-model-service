// Command minio-provision performs the explicit, one-time Model bucket
// provisioning step. The Model service never invokes this command itself.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func main() {
	endpoint := strings.TrimSpace(os.Getenv("ANI_MINIO_ENDPOINT"))
	accessKey := os.Getenv("ANI_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("ANI_MINIO_SECRET_KEY")
	bucket := strings.TrimSpace(os.Getenv("ANI_MINIO_BUCKET"))
	if tenantID := strings.TrimSpace(os.Getenv("ANI_MINIO_TENANT_ID")); tenantID != "" {
		parsed, err := uuid.Parse(tenantID)
		if err != nil {
			fatal("ANI_MINIO_TENANT_ID must be a UUID: %v", err)
		}
		bucket = strings.ToLower(parsed.String())
	}
	if bucket == "" {
		bucket = "ani-models"
	}
	if endpoint == "" || accessKey == "" || secretKey == "" {
		fatal("ANI_MINIO_ENDPOINT, ANI_MINIO_ACCESS_KEY and ANI_MINIO_SECRET_KEY are required")
	}
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: os.Getenv("ANI_MINIO_SECURE") == "true"})
	if err != nil {
		fatal("create MinIO client: %v", err)
	}
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		fatal("check bucket %q: %v", bucket, err)
	}
	if exists {
		fmt.Printf("bucket %s already exists\n", bucket)
		return
	}
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		fatal("create bucket %q: %v", bucket, err)
	}
	fmt.Printf("created bucket %s\n", bucket)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
