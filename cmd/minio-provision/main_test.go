package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/minio/minio-go/v7"
)

type bucketFake struct {
	exists  bool
	created int
}

func (f *bucketFake) BucketExists(context.Context, string) (bool, error) { return f.exists, nil }
func (f *bucketFake) MakeBucket(context.Context, string, minio.MakeBucketOptions) error {
	f.created++
	f.exists = true
	return nil
}
func (f *bucketFake) PutObject(context.Context, string, string, io.Reader, int64, minio.PutObjectOptions) (minio.UploadInfo, error) {
	return minio.UploadInfo{}, nil
}

func TestEnsureBucketCreatesOnlyWhenMissing(t *testing.T) {
	fake := &bucketFake{}
	if err := ensureBucket(context.Background(), fake, "tenant"); err != nil {
		t.Fatal(err)
	}
	if fake.created != 1 {
		t.Fatalf("created = %d, want 1", fake.created)
	}
	if err := ensureBucket(context.Background(), fake, "tenant"); err != nil {
		t.Fatal(err)
	}
	if fake.created != 1 {
		t.Fatalf("created = %d after existing bucket, want 1", fake.created)
	}
}

func TestDownloadCommandModelScope(t *testing.T) {
	get := func(key string) string {
		if key == "ANI_MODELSCOPE_CLI" {
			return "/usr/local/bin/modelscope"
		}
		return ""
	}
	name, args, err := downloadCommand("modelscope", "Qwen/Qwen3#config.json", "v1", "/staging/model", get)
	if err != nil {
		t.Fatal(err)
	}
	if name != "/usr/local/bin/modelscope" {
		t.Fatalf("name = %q", name)
	}
	want := []string{"download", "--model", "Qwen/Qwen3", "--revision", "v1", "--local_dir", "/staging/model", "config.json"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestDefaultRevisionIsProviderSpecific(t *testing.T) {
	if got := defaultRevision("modelscope", ""); got != "master" {
		t.Fatalf("ModelScope revision = %q, want master", got)
	}
	if got := defaultRevision("huggingface", ""); got != "main" {
		t.Fatalf("Hugging Face revision = %q, want main", got)
	}
}

func TestArchiveDirectoryRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("config.json", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := archiveDirectory(root, io.Discard); err == nil {
		t.Fatal("archiveDirectory() error = nil")
	}
}

func TestObjectKeyRequiresTaskOrExplicitKey(t *testing.T) {
	if _, err := objectKey("", ""); err == nil {
		t.Fatal("objectKey() error = nil")
	}
	key, err := objectKey("tenant/model.tar", "")
	if err != nil || key != "tenant/model.tar" {
		t.Fatalf("objectKey() = %q, %v", key, err)
	}
}

func TestBucketNameHonorsSharedBucketMode(t *testing.T) {
	tenant := "99999999-9999-4999-8999-999999999999"
	if got, err := bucketName(tenant, "ani-models", false); err != nil || got != "ani-models" {
		t.Fatalf("shared bucketName() = %q, %v", got, err)
	}
	if got, err := bucketName(tenant, "ani-models", true); err != nil || got != tenant {
		t.Fatalf("tenant bucketName() = %q, %v", got, err)
	}
}

func TestStorageObjectKeyPrefixesSharedBucketTenant(t *testing.T) {
	tenant := "99999999-9999-4999-8999-999999999999"
	if got := storageObjectKey(tenant, "imports/task/model.tar", false); got != tenant+"/imports/task/model.tar" {
		t.Fatalf("shared storageObjectKey() = %q", got)
	}
	if got := storageObjectKey(tenant, "imports/task/model.tar", true); got != "imports/task/model.tar" {
		t.Fatalf("tenant storageObjectKey() = %q", got)
	}
}
