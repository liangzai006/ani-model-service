package storage

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	bizstorage "github.com/liangzai006/ani-model-service/internal/biz/storage"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type bucketAPIFake struct {
	exists  bool
	checks  int
	creates int
	err     error
}

func TestMinIOUploadURLTTL(t *testing.T) {
	client, err := minio.New("minio.example:9000", &minio.Options{
		Creds:  credentials.NewStaticV4("test-access", "test-secret", ""),
		Region: "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &MinIOAdapter{client: client, bucket: "models"}
	for _, test := range []struct {
		name string
		ttl  time.Duration
		want string
	}{
		{name: "ordinary upload default", want: "900"},
		{name: "configured import upload", ttl: time.Hour, want: "3600"},
	} {
		t.Run(test.name, func(t *testing.T) {
			signed, err := adapter.CreateUploadURL(context.Background(), bizstorage.UploadRequest{TenantID: "tenant-a", ObjectRef: "imports/task/model.tar", TTL: test.ttl})
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := url.Parse(signed.URL)
			if err != nil {
				t.Fatal(err)
			}
			if got := parsed.Query().Get("X-Amz-Expires"); got != test.want {
				t.Fatalf("signed upload expiry = %q seconds, want %q", got, test.want)
			}
		})
	}
}

func (f *bucketAPIFake) BucketExists(context.Context, string) (bool, error) {
	f.checks++
	return f.exists, f.err
}
func (f *bucketAPIFake) MakeBucket(context.Context, string, minio.MakeBucketOptions) error {
	f.creates++
	if f.err == nil {
		f.exists = true
	}
	return f.err
}

func TestTenantKeyScopesReferences(t *testing.T) {
	got, err := tenantKey("tenant-a", "model/v1/file.gguf")
	if err != nil || got != "tenant-a/model/v1/file.gguf" {
		t.Fatalf("key=%q err=%v", got, err)
	}
	got, err = tenantKey("tenant-a", "tenant-a/model/v1/file.gguf")
	if err != nil || got != "tenant-a/model/v1/file.gguf" {
		t.Fatalf("already scoped key=%q err=%v", got, err)
	}
	tenant := uuid.NewString()
	if _, err := tenantKey(tenant, uuid.NewString()+"/model/file"); err == nil {
		t.Fatal("cross-tenant key accepted")
	}
}

func TestNewMinIOAdapterRequiresConfiguration(t *testing.T) {
	if _, err := NewMinIOAdapter("", "access", "secret", "models", false); err == nil {
		t.Fatal("missing endpoint accepted")
	}
	if _, err := NewMinIOAdapter("minio:9000", "access", "secret", "", false); err == nil {
		t.Fatal("missing bucket accepted")
	}
}

func TestTenantBucketUsesValidatedUUID(t *testing.T) {
	tenant := uuid.NewString()
	got, err := tenantBucket(tenant)
	if err != nil || got != tenant {
		t.Fatalf("bucket=%q err=%v", got, err)
	}
	if _, err := tenantBucket("tenant-a"); err == nil {
		t.Fatal("non-UUID tenant accepted as bucket")
	}
}

func TestEnsureBucketDoesNotCreateExistingTenantBucket(t *testing.T) {
	fake := &bucketAPIFake{exists: true}
	a := &MinIOAdapter{buckets: fake, tenantBuckets: true}
	if err := a.EnsureBucket(context.Background(), uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if fake.checks != 1 || fake.creates != 0 {
		t.Fatalf("checks=%d creates=%d", fake.checks, fake.creates)
	}
}

func TestEnsureBucketCreatesMissingTenantBucket(t *testing.T) {
	fake := &bucketAPIFake{}
	a := &MinIOAdapter{buckets: fake, tenantBuckets: true}
	if err := a.EnsureBucket(context.Background(), uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if fake.checks != 1 || fake.creates != 1 || !fake.exists {
		t.Fatalf("checks=%d creates=%d exists=%v", fake.checks, fake.creates, fake.exists)
	}
}

func TestEnsureBucketRejectsInvalidTenant(t *testing.T) {
	a := &MinIOAdapter{buckets: &bucketAPIFake{}, tenantBuckets: true}
	if err := a.EnsureBucket(context.Background(), "tenant-a"); err == nil || !errors.Is(err, errInvalidTenantBucket) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckTenantConnectionAllowsMissingBucket(t *testing.T) {
	fake := &bucketAPIFake{}
	a := &MinIOAdapter{buckets: fake, tenantBuckets: true}
	if err := a.CheckTenantConnection(context.Background(), uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if fake.checks != 1 || fake.creates != 0 {
		t.Fatalf("checks=%d creates=%d", fake.checks, fake.creates)
	}
}
