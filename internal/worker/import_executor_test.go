package worker

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
	"github.com/liangzai006/ani-model-service/internal/biz/storage"
	workbiz "github.com/liangzai006/ani-model-service/internal/biz/work"
	"github.com/liangzai006/ani-model-service/internal/data/importer"
)

type manifestSourceFake struct{}

func (manifestSourceFake) Fetch(context.Context, importer.Request) (importer.Result, error) {
	return importer.Result{}, errors.New("manifest source requires bundle path")
}
func (manifestSourceFake) ResolveMetadata(context.Context, importer.Request) (importer.Metadata, error) {
	return importer.Metadata{ExternalModelID: "model", Version: "main"}, nil
}
func (manifestSourceFake) ListFiles(context.Context, importer.Request) ([]importer.File, error) {
	return []importer.File{{Path: "config.json", Size: 2}, {Path: "weights.bin", Size: 3}}, nil
}
func (manifestSourceFake) FetchContent(_ context.Context, req importer.Request) (importer.ContentResult, error) {
	if strings.HasSuffix(req.RepoID, "config.json") {
		return importer.ContentResult{Result: importer.Result{ObjectRef: "org/model/main/config.json", SizeBytes: 2}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	}
	return importer.ContentResult{Result: importer.Result{ObjectRef: "org/model/main/weights.bin", SizeBytes: 3}, Body: io.NopCloser(strings.NewReader("123"))}, nil
}

type sourceFake struct{ result importer.Result }

func (s sourceFake) Fetch(context.Context, importer.Request) (importer.Result, error) {
	return s.result, nil
}

type contentSourceFake struct{}

func (contentSourceFake) Fetch(context.Context, importer.Request) (importer.Result, error) {
	return importer.Result{}, errors.New("unexpected metadata fetch")
}

type recordingContentSource struct {
	called bool
	contentSourceFake
}

func (s *recordingContentSource) FetchContent(ctx context.Context, req importer.Request) (importer.ContentResult, error) {
	s.called = true
	return s.contentSourceFake.FetchContent(ctx, req)
}
func (contentSourceFake) FetchContent(context.Context, importer.Request) (importer.ContentResult, error) {
	return importer.ContentResult{Result: importer.Result{ObjectRef: "org/model/main/model.gguf", Format: "gguf", SizeBytes: 11}, Body: io.NopCloser(strings.NewReader("model-bytes")), ContentType: "application/octet-stream"}, nil
}

type objectFake struct {
	exists    bool
	verified  bool
	uploaded  string
	ensureErr error
	ensured   bool
}

type artifactStoreFake struct{ artifact modelbiz.Artifact }

type checksumStoreFake struct {
	checksum string
	size     int64
	err      error
	calls    int
}

func (s *checksumStoreFake) SetChecksum(_ context.Context, _, _, checksum string, size int64) error {
	s.calls++
	s.checksum, s.size = checksum, size
	return s.err
}

func (s *artifactStoreFake) CreateArtifact(_ context.Context, a modelbiz.Artifact) (modelbiz.Artifact, error) {
	s.artifact = a
	return a, nil
}
func (s *artifactStoreFake) GetArtifact(context.Context, string, string) (modelbiz.Artifact, error) {
	return s.artifact, nil
}

func (s *objectFake) CreateUploadURL(context.Context, storage.UploadRequest) (storage.SignedURL, error) {
	return storage.SignedURL{}, nil
}
func (s *objectFake) ObjectExists(context.Context, string, string) (bool, error) {
	return s.exists, nil
}
func (s *objectFake) VerifyChecksum(context.Context, string, string, string) error {
	s.verified = true
	return nil
}
func (s *objectFake) CreateDownloadURL(context.Context, storage.DownloadRequest) (storage.SignedURL, error) {
	return storage.SignedURL{}, nil
}
func (s *objectFake) EnsureBucket(context.Context, string) error {
	s.ensured = true
	return s.ensureErr
}
func (s *objectFake) Upload(_ context.Context, _ storage.UploadRequest, body io.Reader) error {
	data, err := io.ReadAll(body)
	s.uploaded = string(data)
	return err
}

func TestImportExecutorChecksObjectAndChecksum(t *testing.T) {
	sf := &objectFake{exists: true}
	e := ImportExecutor{Providers: importer.Registry{"huggingface": sourceFake{importer.Result{ObjectRef: "obj", ChecksumSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}}, Storage: sf}
	err := e.Execute(context.Background(), workbiz.Task{TenantID: "t", Source: "huggingface", RepoID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if !sf.verified {
		t.Fatal("checksum was not verified")
	}
}
func TestImportExecutorRejectsMissingObject(t *testing.T) {
	e := ImportExecutor{Providers: importer.Registry{"modelscope": sourceFake{importer.Result{ObjectRef: "obj", ChecksumSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}}, Storage: &objectFake{}}
	if !errors.Is(e.Execute(context.Background(), workbiz.Task{TenantID: "t", Source: "modelscope"}), ErrObjectMissing) {
		t.Fatal("missing object accepted")
	}
}

func TestImportExecutorStreamsProviderContentToStorage(t *testing.T) {
	sf := &objectFake{exists: true}
	e := ImportExecutor{Providers: importer.Registry{"huggingface": contentSourceFake{}}, Storage: sf}
	if err := e.Execute(context.Background(), workbiz.Task{TenantID: "tenant", Source: "huggingface", RepoID: "org/model#model.gguf"}); err != nil {
		t.Fatal(err)
	}
	if sf.uploaded != "model-bytes" || !sf.verified {
		t.Fatalf("uploaded=%q verified=%v", sf.uploaded, sf.verified)
	}
}

func TestImportExecutorPersistsArtifactForBoundVersion(t *testing.T) {
	sf := &objectFake{exists: true}
	artifacts := &artifactStoreFake{}
	e := ImportExecutor{Providers: importer.Registry{"huggingface": contentSourceFake{}}, Storage: sf, Artifacts: artifacts}
	task := workbiz.Task{TenantID: "tenant", ID: "11111111-1111-1111-1111-111111111111", VersionID: "22222222-2222-2222-2222-222222222222", Source: "huggingface", RepoID: "org/model#model.gguf"}
	if err := e.Execute(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if artifacts.artifact.ModelVersionID != task.VersionID || artifacts.artifact.Reference != "tenant/org/model/main/model.gguf" || len(artifacts.artifact.SHA256) != 64 {
		t.Fatalf("artifact = %+v", artifacts.artifact)
	}
}

func TestImportExecutorPersistsComputedChecksumForUnspecifiedVersion(t *testing.T) {
	sf := &objectFake{exists: true}
	artifacts := &artifactStoreFake{}
	checksums := &checksumStoreFake{}
	e := ImportExecutor{Providers: importer.Registry{"huggingface": contentSourceFake{}}, Storage: sf, Artifacts: artifacts, Checksums: checksums}
	task := workbiz.Task{TenantID: "tenant", ID: "11111111-1111-1111-1111-111111111111", VersionID: "22222222-2222-2222-2222-222222222222", Source: "huggingface", RepoID: "org/model#model.gguf"}
	if err := e.Execute(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if len(checksums.checksum) != 64 || checksums.size != int64(len("model-bytes")) {
		t.Fatalf("checksum update = %+v", checksums)
	}
}

func TestImportExecutorReplaysChecksumForExistingVersion(t *testing.T) {
	sf := &objectFake{exists: true}
	artifacts := &artifactStoreFake{}
	checksums := &checksumStoreFake{}
	e := ImportExecutor{Providers: importer.Registry{"huggingface": sourceFake{importer.Result{ObjectRef: "obj", ChecksumSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}}, Storage: sf, Artifacts: artifacts, Checksums: checksums}
	task := workbiz.Task{TenantID: "tenant", ID: "11111111-1111-1111-1111-111111111111", VersionID: "22222222-2222-2222-2222-222222222222", Source: "huggingface", RepoID: "org/model"}
	if err := e.Execute(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if checksums.calls != 1 {
		t.Fatalf("checksum writes=%d, want one idempotent replay", checksums.calls)
	}
}

func TestImportExecutorEnsuresTenantBucketBeforeDownload(t *testing.T) {
	sf := &objectFake{exists: true}
	e := ImportExecutor{Providers: importer.Registry{"huggingface": contentSourceFake{}}, Storage: sf}
	if err := e.Execute(context.Background(), workbiz.Task{TenantID: "tenant", Source: "huggingface", RepoID: "org/model#model.gguf"}); err != nil {
		t.Fatal(err)
	}
	if !sf.ensured {
		t.Fatal("tenant bucket was not ensured")
	}
}

func TestImportExecutorArchivesRepositoryManifest(t *testing.T) {
	sf := &objectFake{exists: true}
	e := ImportExecutor{Providers: importer.Registry{"huggingface": manifestSourceFake{}}, Storage: sf}
	if err := e.Execute(context.Background(), workbiz.Task{TenantID: "tenant", Source: "huggingface", RepoID: "org/model", Revision: "main"}); err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(strings.NewReader(sf.uploaded))
	seen := map[string]string{}
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		seen[header.Name] = string(body)
	}
	if seen["config.json"] != "{}" || seen["weights.bin"] != "123" || len(seen) != 2 {
		t.Fatalf("archive entries=%v", seen)
	}
}

func TestImportExecutorStopsBeforeDownloadWhenBucketEnsureFails(t *testing.T) {
	source := &recordingContentSource{}
	sf := &objectFake{ensureErr: errors.New("bucket unavailable")}
	e := ImportExecutor{Providers: importer.Registry{"huggingface": source}, Storage: sf}
	if err := e.Execute(context.Background(), workbiz.Task{TenantID: "tenant", Source: "huggingface", RepoID: "org/model#model.gguf"}); err == nil {
		t.Fatal("download succeeded with unavailable tenant bucket")
	}
	if source.called {
		t.Fatal("provider was called before bucket ensure")
	}
}

func TestImportExecutorLogsDownloadLifecycle(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	sf := &objectFake{exists: true}
	e := ImportExecutor{Logger: logger, Providers: importer.Registry{"huggingface": contentSourceFake{}}, Storage: sf}
	task := workbiz.Task{TenantID: "tenant", ID: "task-id", Source: "huggingface", RepoID: "org/model#model.gguf", Revision: "main"}
	if err := e.Execute(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	got := logs.String()
	for _, want := range []string{"provider download started", "provider download completed", "storage upload completed", "task-id", "huggingface"} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output %q does not contain %q", got, want)
		}
	}
}
