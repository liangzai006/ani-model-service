package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRunRequiresUploadURL(t *testing.T) {
	get := func(key string) string {
		if key == "ANI_IMPORT_SOURCE" {
			return "modelscope"
		}
		if key == "ANI_IMPORT_REPO_ID" {
			return "org/model"
		}
		return ""
	}
	if err := run(context.Background(), get, func(context.Context, string, ...string) *exec.Cmd { return nil }); err == nil || !strings.Contains(err.Error(), "ANI_IMPORT_UPLOAD_URL") {
		t.Fatalf("run() error = %v, want missing upload URL", err)
	}
}

func TestRunUploadsArchiveThroughSignedURL(t *testing.T) {
	staging := t.TempDir()
	if err := os.WriteFile(filepath.Join(staging, "weights.safetensors"), []byte("weights"), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotMethod string
	var gotLength int64
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotLength = r.ContentLength
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	get := func(key string) string {
		values := map[string]string{
			"ANI_IMPORT_UPLOAD_URL":  server.URL,
			"ANI_IMPORT_SOURCE":      "modelscope",
			"ANI_IMPORT_REPO_ID":     "org/model",
			"ANI_IMPORT_STAGING_DIR": staging,
			"ANI_IMPORT_OBJECT_KEY":  "imports/task/model.tar",
		}
		return values[key]
	}
	command := func(ctx context.Context, _ string, _ ...string) *exec.Cmd { return exec.CommandContext(ctx, "true") }
	if err := run(context.Background(), get, command); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("method = %q, want PUT", gotMethod)
	}
	if gotLength <= 0 || gotLength != int64(len(gotBody)) {
		t.Fatalf("content length = %d, body bytes = %d", gotLength, len(gotBody))
	}
	if !bytes.Contains(gotBody, []byte("weights.safetensors")) {
		t.Fatalf("archive body does not contain staged file name")
	}
}

func TestRunRejectsUploadFailure(t *testing.T) {
	staging := t.TempDir()
	if err := os.WriteFile(filepath.Join(staging, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	get := func(key string) string {
		values := map[string]string{
			"ANI_IMPORT_UPLOAD_URL":  server.URL,
			"ANI_IMPORT_SOURCE":      "modelscope",
			"ANI_IMPORT_REPO_ID":     "org/model",
			"ANI_IMPORT_STAGING_DIR": staging,
			"ANI_IMPORT_OBJECT_KEY":  "imports/task/model.tar",
		}
		return values[key]
	}
	command := func(ctx context.Context, _ string, _ ...string) *exec.Cmd { return exec.CommandContext(ctx, "true") }
	if err := run(context.Background(), get, command); err == nil || !strings.Contains(err.Error(), "upload model archive") {
		t.Fatalf("run() error = %v, want upload failure", err)
	}
}

func TestRunRejectsUploadRedirect(t *testing.T) {
	staging := t.TempDir()
	if err := os.WriteFile(filepath.Join(staging, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	success := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer success.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", success.URL)
		w.WriteHeader(http.StatusFound)
	}))
	defer redirect.Close()
	get := func(key string) string {
		values := map[string]string{
			"ANI_IMPORT_UPLOAD_URL":  redirect.URL,
			"ANI_IMPORT_SOURCE":      "modelscope",
			"ANI_IMPORT_REPO_ID":     "org/model",
			"ANI_IMPORT_STAGING_DIR": staging,
			"ANI_IMPORT_OBJECT_KEY":  "imports/task/model.tar",
		}
		return values[key]
	}
	command := func(ctx context.Context, _ string, _ ...string) *exec.Cmd { return exec.CommandContext(ctx, "true") }
	if err := run(context.Background(), get, command); err == nil || !strings.Contains(err.Error(), "upload model archive") {
		t.Fatalf("run() error = %v, want redirect rejection", err)
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
	want := []string{"download", "--revision", "v1", "--local-dir", "/staging/model", "Qwen/Qwen3", "config.json"}
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
