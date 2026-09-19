package worker

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
	"github.com/liangzai006/ani-model-service/internal/biz/storage"
	workbiz "github.com/liangzai006/ani-model-service/internal/biz/work"
	"github.com/liangzai006/ani-model-service/internal/data/importer"
)

var ErrObjectMissing = errors.New("import object missing")
var ErrArtifactStoreUnavailable = errors.New("import artifact store unavailable")

// ImportExecutor coordinates provider metadata with the external Storage
// service. It does not own buckets, filesystems, or provider credentials.
type ImportExecutor struct {
	Providers importer.Registry
	Storage   storage.Port
	Artifacts modelbiz.ArtifactStore
	Checksums modelbiz.VersionChecksumStore
	Logger    *slog.Logger
}

func (e ImportExecutor) Execute(ctx context.Context, t workbiz.Task) error {
	provider, err := e.Providers.Resolve(t.Source)
	if err != nil {
		return err
	}
	if e.Storage == nil {
		return storage.ErrProvider
	}
	if manager, ok := e.Storage.(storage.BucketManager); ok {
		e.log(ctx, "bucket ensure started", t, slog.String("bucket_scope", "tenant"))
		if err := manager.EnsureBucket(ctx, t.TenantID); err != nil {
			e.log(ctx, "bucket ensure failed", t, slog.String("error_class", errorClass(err)))
			return err
		}
		e.log(ctx, "bucket ensured", t, slog.String("bucket_scope", "tenant"))
	}
	if !strings.Contains(t.RepoID, "#") {
		if manifestSource, ok := provider.(importer.ManifestSource); ok {
			return e.executeManifest(ctx, t, provider, manifestSource)
		}
	}
	if contentSource, ok := provider.(importer.ContentSource); ok {
		e.log(ctx, "provider download started", t, slog.String("provider", t.Source), slog.String("repo_id", t.RepoID), slog.String("revision", t.Revision))
		content, err := contentSource.FetchContent(ctx, importer.Request{RepoID: t.RepoID, Revision: t.Revision})
		if err != nil {
			e.log(ctx, "provider download failed", t, slog.String("provider", t.Source), slog.String("error_class", errorClass(err)))
			return fmt.Errorf("%w: %v", importer.ErrProvider, err)
		}
		if content.Body == nil {
			return storage.ErrProvider
		}
		defer content.Body.Close()
		if content.Result.ObjectRef == "" {
			return storage.ErrProvider
		}
		objectRef := t.TenantID + "/" + strings.TrimLeft(content.Result.ObjectRef, "/")
		if err := e.uploadStream(ctx, t, objectRef, content.Result.Format, content.Result.SizeBytes, content.ContentType, content.Result.ChecksumSHA256, content.Body); err != nil {
			return err
		}
		e.log(ctx, "provider download completed", t, slog.String("provider", t.Source))
		return nil
	}
	r, err := provider.Fetch(ctx, importer.Request{RepoID: t.RepoID, Revision: t.Revision})
	if err != nil {
		e.log(ctx, "provider metadata fetch failed", t, slog.String("provider", t.Source), slog.String("error_class", errorClass(err)))
		return fmt.Errorf("%w: %v", importer.ErrProvider, err)
	}
	e.log(ctx, "provider metadata fetch completed", t, slog.String("provider", t.Source))
	if strings.TrimSpace(r.ObjectRef) == "" || len(r.ChecksumSHA256) != 64 {
		return storage.ErrChecksumMismatch
	}
	ok, err := e.Storage.ObjectExists(ctx, t.TenantID, r.ObjectRef)
	if err != nil {
		return err
	}
	if !ok {
		return ErrObjectMissing
	}
	if err := e.Storage.VerifyChecksum(ctx, t.TenantID, r.ObjectRef, r.ChecksumSHA256); err != nil {
		return err
	}
	if err := e.persistArtifact(ctx, t, r); err != nil {
		return err
	}
	if e.Checksums != nil {
		if err := e.Checksums.SetChecksum(ctx, t.TenantID, t.VersionID, r.ChecksumSHA256, r.SizeBytes); err != nil {
			return err
		}
	}
	return nil
}

const maxImportBundleBytes = int64(512 << 20)

func (e ImportExecutor) executeManifest(ctx context.Context, t workbiz.Task, provider importer.SourceAdapter, manifestSource importer.ManifestSource) error {
	files, err := manifestSource.ListFiles(ctx, importer.Request{RepoID: t.RepoID, Revision: t.Revision})
	if err != nil {
		return err
	}
	contentSource, ok := provider.(importer.ContentSource)
	if !ok {
		return fmt.Errorf("%w: provider does not expose repository files", importer.ErrProvider)
	}
	revision := t.Revision
	if revision == "" {
		revision = "main"
	}
	repo := strings.Trim(t.RepoID, "/")
	if err := validateArchivePath(repo); err != nil || strings.Contains(repo, "#") {
		return fmt.Errorf("%w: invalid repository reference", importer.ErrProvider)
	}
	if err := validateArchivePath(revision); err != nil {
		return fmt.Errorf("%w: invalid repository revision", importer.ErrProvider)
	}
	objectRef := fmt.Sprintf("%s/%s/%s/model.tar", t.TenantID, repo, revision)
	pipeReader, pipeWriter := io.Pipe()
	go func() {
		tw := tar.NewWriter(pipeWriter)
		var total int64
		closeWithError := func(err error) {
			_ = tw.Close()
			_ = pipeWriter.CloseWithError(err)
		}
		for _, file := range files {
			if err := validateArchivePath(file.Path); err != nil {
				closeWithError(err)
				return
			}
			if file.Size > 0 && total+file.Size > maxImportBundleBytes {
				closeWithError(fmt.Errorf("%w: repository exceeds %d bytes", importer.ErrProvider, maxImportBundleBytes))
				return
			}
			content, err := contentSource.FetchContent(ctx, importer.Request{RepoID: repo + "#" + file.Path, Revision: revision})
			if err != nil || content.Body == nil {
				if err == nil {
					err = storage.ErrProvider
				}
				closeWithError(err)
				return
			}
			func() {
				defer content.Body.Close()
				var data []byte
				if file.Size < 0 {
					data, err = io.ReadAll(io.LimitReader(content.Body, maxImportBundleBytes-total+1))
					file.Size = int64(len(data))
					if err == nil && file.Size+total > maxImportBundleBytes {
						err = fmt.Errorf("%w: repository exceeds %d bytes", importer.ErrProvider, maxImportBundleBytes)
					}
				}
				if err != nil {
					return
				}
				if err = tw.WriteHeader(&tar.Header{Name: path.Clean(file.Path), Mode: 0644, Size: file.Size}); err != nil {
					return
				}
				if data != nil {
					_, err = tw.Write(data)
				} else {
					var copied int64
					copied, err = io.CopyN(tw, content.Body, file.Size)
					if err == nil && copied != file.Size {
						err = io.ErrUnexpectedEOF
					}
				}
				if err == nil {
					total += file.Size
				}
			}()
			if err != nil {
				closeWithError(err)
				return
			}
		}
		if err := tw.Close(); err != nil {
			_ = pipeWriter.CloseWithError(err)
			return
		}
		_ = pipeWriter.Close()
	}()
	return e.uploadStream(ctx, t, objectRef, bundleFormat(files), -1, "application/x-tar", "", pipeReader)
}

func (e ImportExecutor) uploadStream(ctx context.Context, t workbiz.Task, objectRef, format string, size int64, contentType, expected string, body io.Reader) error {
	uploader, ok := e.Storage.(storage.Uploader)
	if !ok {
		return storage.ErrProvider
	}
	hasher := sha256.New()
	var bytesRead int64
	if err := uploader.Upload(ctx, storage.UploadRequest{TenantID: t.TenantID, ObjectRef: objectRef, ContentType: contentType, SizeBytes: size}, io.TeeReader(body, io.MultiWriter(hasher, countWriter{n: &bytesRead}))); err != nil {
		e.log(ctx, "storage upload failed", t, slog.String("error_class", errorClass(err)))
		return err
	}
	exists, err := e.Storage.ObjectExists(ctx, t.TenantID, objectRef)
	if err != nil {
		return err
	}
	if !exists {
		return ErrObjectMissing
	}
	checksum := expected
	if checksum == "" {
		checksum = hex.EncodeToString(hasher.Sum(nil))
	}
	if err := e.Storage.VerifyChecksum(ctx, t.TenantID, objectRef, checksum); err != nil {
		e.log(ctx, "storage checksum verification failed", t, slog.String("error_class", errorClass(err)))
		return err
	}
	e.log(ctx, "storage upload completed", t, slog.Int64("bytes", bytesRead))
	result := importer.Result{ObjectRef: objectRef, Format: format, SizeBytes: bytesRead, ChecksumSHA256: checksum}
	if err := e.persistArtifact(ctx, t, result); err != nil {
		return err
	}
	if e.Checksums != nil {
		if err := e.Checksums.SetChecksum(ctx, t.TenantID, t.VersionID, checksum, result.SizeBytes); err != nil {
			return err
		}
	}
	return nil
}

func validateArchivePath(name string) error {
	if name == "" || strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") || path.IsAbs(name) || path.Clean(name) != name || name == "." || strings.HasPrefix(name, "../") {
		return fmt.Errorf("%w: unsafe repository path", importer.ErrProvider)
	}
	return nil
}

func bundleFormat(files []importer.File) string {
	for _, file := range files {
		if strings.HasSuffix(strings.ToLower(file.Path), ".gguf") {
			return "gguf"
		}
	}
	return "safetensors"
}

func (e ImportExecutor) log(ctx context.Context, message string, t workbiz.Task, attrs ...any) {
	if e.Logger == nil {
		return
	}
	base := []any{slog.String("tenant_id", t.TenantID), slog.String("task_id", t.ID)}
	e.Logger.InfoContext(ctx, message, append(base, attrs...)...)
}

func (e ImportExecutor) persistArtifact(ctx context.Context, t workbiz.Task, r importer.Result) error {
	if t.VersionID == "" {
		return nil
	}
	if e.Artifacts == nil {
		return ErrArtifactStoreUnavailable
	}
	format := r.Format
	if format == "" {
		format = "pytorch"
	}
	want := modelbiz.Artifact{
		TenantID: t.TenantID, ID: t.ID, ModelVersionID: t.VersionID,
		Provider: t.Source, Reference: r.ObjectRef, Format: format,
		SizeBytes: r.SizeBytes, SHA256: r.ChecksumSHA256,
	}
	_, err := e.Artifacts.CreateArtifact(ctx, want)
	if err == nil {
		return nil
	}
	// A retry after a worker crash may have persisted the artifact already.
	// Treat the unique-key replay as success only when immutable facts match.
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "23505" {
		return err
	}
	existing, getErr := e.Artifacts.GetArtifact(ctx, t.TenantID, t.VersionID)
	if getErr != nil {
		return err
	}
	if existing.Reference != want.Reference || existing.SHA256 != want.SHA256 || existing.Format != want.Format || existing.SizeBytes != want.SizeBytes {
		return storage.ErrChecksumMismatch
	}
	return nil
}

type countWriter struct{ n *int64 }

func (w countWriter) Write(p []byte) (int, error) { *w.n += int64(len(p)); return len(p), nil }

func sizeOrCount(size, count int64) int64 {
	if size >= 0 {
		return size
	}
	return count
}
