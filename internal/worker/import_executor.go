package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	modelbiz "github.com/zhangzhe-ctrl/ani-model-service/internal/biz/model"
	"github.com/zhangzhe-ctrl/ani-model-service/internal/biz/storage"
	workbiz "github.com/zhangzhe-ctrl/ani-model-service/internal/biz/work"
	"github.com/zhangzhe-ctrl/ani-model-service/internal/data/importer"
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
		uploader, ok := e.Storage.(storage.Uploader)
		if !ok {
			return storage.ErrProvider
		}
		hasher := sha256.New()
		var bytesRead int64
		body := io.TeeReader(content.Body, io.MultiWriter(hasher, countWriter{n: &bytesRead}))
		if err := uploader.Upload(ctx, storage.UploadRequest{TenantID: t.TenantID, ObjectRef: objectRef, ContentType: content.ContentType, SizeBytes: content.Result.SizeBytes}, body); err != nil {
			e.log(ctx, "storage upload failed", t, slog.String("error_class", errorClass(err)))
			return err
		}
		e.log(ctx, "provider download completed", t, slog.String("provider", t.Source), slog.Int64("bytes", bytesRead))
		exists, err := e.Storage.ObjectExists(ctx, t.TenantID, objectRef)
		if err != nil {
			return err
		}
		if !exists {
			return ErrObjectMissing
		}
		checksum := content.Result.ChecksumSHA256
		if checksum == "" {
			checksum = hex.EncodeToString(hasher.Sum(nil))
		}
		if err := e.Storage.VerifyChecksum(ctx, t.TenantID, objectRef, checksum); err != nil {
			e.log(ctx, "storage checksum verification failed", t, slog.String("error_class", errorClass(err)))
			return err
		}
		e.log(ctx, "storage upload completed", t, slog.Int64("bytes", sizeOrCount(content.Result.SizeBytes, bytesRead)))
		result := importer.Result{ObjectRef: objectRef, Format: content.Result.Format, SizeBytes: sizeOrCount(content.Result.SizeBytes, bytesRead), ChecksumSHA256: checksum}
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
