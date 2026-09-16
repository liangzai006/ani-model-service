package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

type bucketAPIFake struct {
	exists  bool
	checks  int
	creates int
	err     error
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
