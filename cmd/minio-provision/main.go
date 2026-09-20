// Command minio-provision is the one-shot model import entrypoint used by a
// Kubernetes Job. It first ensures the tenant bucket exists, then downloads
// the requested repository with the provider CLI and uploads one immutable
// archive to MinIO.
package main

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type bucketClient interface {
	BucketExists(context.Context, string) (bool, error)
	MakeBucket(context.Context, string, minio.MakeBucketOptions) error
	PutObject(context.Context, string, string, io.Reader, int64, minio.PutObjectOptions) (minio.UploadInfo, error)
}

type importResult struct {
	ObjectRef string `json:"object_ref"`
	Format    string `json:"format"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

func main() {
	if err := run(context.Background(), os.Getenv, exec.CommandContext); err != nil {
		fatal("model import failed: %v", err)
	}
}

type getenv func(string) string
type commandFactory func(context.Context, string, ...string) *exec.Cmd

func run(ctx context.Context, get getenv, command commandFactory) error {
	endpoint := strings.TrimSpace(get("ANI_MINIO_ENDPOINT"))
	accessKey := get("ANI_MINIO_ACCESS_KEY")
	secretKey := get("ANI_MINIO_SECRET_KEY")
	if endpoint == "" || accessKey == "" || secretKey == "" {
		return errors.New("ANI_MINIO_ENDPOINT, ANI_MINIO_ACCESS_KEY and ANI_MINIO_SECRET_KEY are required")
	}
	tenantID := strings.TrimSpace(firstNonEmpty(get("ANI_IMPORT_TENANT_ID"), get("ANI_MINIO_TENANT_ID")))
	tenantBuckets := strings.EqualFold(strings.TrimSpace(get("ANI_MINIO_TENANT_BUCKETS")), "true")
	bucket, err := bucketName(tenantID, get("ANI_MINIO_BUCKET"), tenantBuckets)
	if err != nil {
		return err
	}
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: strings.EqualFold(get("ANI_MINIO_SECURE"), "true")})
	if err != nil {
		return fmt.Errorf("create MinIO client: %w", err)
	}
	if err := ensureBucket(ctx, client, bucket); err != nil {
		return err
	}

	source := strings.ToLower(strings.TrimSpace(get("ANI_IMPORT_SOURCE")))
	repoID := strings.TrimSpace(get("ANI_IMPORT_REPO_ID"))
	if source == "" || repoID == "" {
		return errors.New("ANI_IMPORT_SOURCE and ANI_IMPORT_REPO_ID are required")
	}
	revision := defaultRevision(source, get("ANI_IMPORT_REVISION"))
	staging := firstNonEmpty(strings.TrimSpace(get("ANI_IMPORT_STAGING_DIR")), "/staging/model")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	name, args, err := downloadCommand(source, repoID, revision, staging, get)
	if err != nil {
		return err
	}
	cmd := command(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("download model: %w", err)
	}

	key, err := objectKey(get("ANI_IMPORT_OBJECT_KEY"), get("ANI_IMPORT_TASK_ID"))
	if err != nil {
		return err
	}
	uploadKey := storageObjectKey(tenantID, key, tenantBuckets)
	result, err := uploadArchive(ctx, client, bucket, key, uploadKey, staging)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(result)
	// The Job controller consumes this stable, single-line result. It contains
	// no credentials and is safe to retain in the Job log for reconciliation.
	fmt.Printf("ANI_IMPORT_RESULT %s\n", encoded)
	return nil
}

func bucketName(tenantID, configured string, tenantBuckets bool) (string, error) {
	if !tenantBuckets || tenantID == "" {
		if configured == "" {
			return "ani-models", nil
		}
		return configured, nil
	}
	parsed, err := uuid.Parse(tenantID)
	if err != nil {
		return "", fmt.Errorf("tenant id must be a UUID: %w", err)
	}
	return strings.ToLower(parsed.String()), nil
}

func storageObjectKey(tenantID, objectRef string, tenantBuckets bool) string {
	if tenantBuckets || tenantID == "" {
		return objectRef
	}
	return strings.Trim(tenantID, "/") + "/" + strings.TrimLeft(objectRef, "/")
}

func ensureBucket(ctx context.Context, client bucketClient, bucket string) error {
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("check bucket %q: %w", bucket, err)
	}
	if exists {
		return nil
	}
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		// A concurrent Job may have created the bucket between the check and
		// create. Verify the final state instead of failing the import.
		exists, checkErr := client.BucketExists(ctx, bucket)
		if checkErr == nil && exists {
			return nil
		}
		return fmt.Errorf("create bucket %q: %w", bucket, err)
	}
	return nil
}

func downloadCommand(source, repoID, revision, staging string, get getenv) (string, []string, error) {
	repo, file, err := splitRepoFile(repoID)
	if err != nil {
		return "", nil, err
	}
	switch source {
	case "modelscope":
		bin := firstNonEmpty(strings.TrimSpace(get("ANI_MODELSCOPE_CLI")), "modelscope")
		args := []string{"download", "--model", repo, "--revision", revision, "--local_dir", staging}
		if file != "" {
			args = append(args, file)
		}
		return bin, args, nil
	case "huggingface":
		bin := firstNonEmpty(strings.TrimSpace(get("ANI_HUGGINGFACE_CLI")), "hf")
		args := []string{"download", repo, "--revision", revision, "--local-dir", staging}
		if file != "" {
			args = append(args, file)
		}
		return bin, args, nil
	default:
		return "", nil, fmt.Errorf("unsupported import source %q", source)
	}
}

func defaultRevision(source, configured string) string {
	if revision := strings.TrimSpace(configured); revision != "" {
		return revision
	}
	if source == "modelscope" {
		return "master"
	}
	return "main"
}

func splitRepoFile(value string) (string, string, error) {
	parts := strings.SplitN(value, "#", 2)
	if strings.TrimSpace(parts[0]) == "" || strings.Contains(parts[0], "..") {
		return "", "", errors.New("invalid repository id")
	}
	if len(parts) == 1 {
		return strings.Trim(parts[0], "/"), "", nil
	}
	if strings.TrimSpace(parts[1]) == "" || strings.Contains(parts[1], "..") || strings.HasPrefix(parts[1], "/") {
		return "", "", errors.New("invalid repository file")
	}
	return strings.Trim(parts[0], "/"), strings.Trim(parts[1], "/"), nil
}

func objectKey(configured, taskID string) (string, error) {
	if configured = strings.Trim(strings.TrimSpace(configured), "/"); configured != "" {
		if strings.Contains(configured, "..") || strings.ContainsRune(configured, '\x00') {
			return "", errors.New("ANI_IMPORT_OBJECT_KEY contains an unsafe path")
		}
		return configured, nil
	}
	if taskID == "" {
		return "", errors.New("ANI_IMPORT_OBJECT_KEY or ANI_IMPORT_TASK_ID is required")
	}
	if _, err := uuid.Parse(taskID); err != nil {
		return "", fmt.Errorf("ANI_IMPORT_TASK_ID must be a UUID: %w", err)
	}
	return "imports/" + taskID + "/model.tar", nil
}

func uploadArchive(ctx context.Context, client bucketClient, bucket, objectRef, uploadKey, root string) (importResult, error) {
	pr, pw := io.Pipe()
	hasher := sha256.New()
	var size int64
	write := io.MultiWriter(pw, hasher, countWriter{n: &size})
	errCh := make(chan error, 1)
	go func() {
		err := archiveDirectory(root, write)
		_ = pw.CloseWithError(err)
		errCh <- err
	}()
	if _, err := client.PutObject(ctx, bucket, uploadKey, pr, -1, minio.PutObjectOptions{ContentType: "application/x-tar"}); err != nil {
		_ = pr.CloseWithError(err)
		return importResult{}, fmt.Errorf("upload model archive: %w", err)
	}
	if err := <-errCh; err != nil {
		return importResult{}, fmt.Errorf("archive model: %w", err)
	}
	return importResult{ObjectRef: objectRef, Format: archiveFormat(root), SizeBytes: size, SHA256: hex.EncodeToString(hasher.Sum(nil))}, nil
}

func archiveFormat(root string) string {
	format := "safetensors"
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".gguf") {
			format = "gguf"
		}
		return nil
	})
	return format
}

func archiveDirectory(root string, out io.Writer) error {
	tw := tar.NewWriter(out)
	defer tw.Close()
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported staging entry %q", path)
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name = filepath.ToSlash(name)
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		info, err := file.Stat()
		if err == nil {
			err = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: info.Size()})
		}
		if err == nil {
			_, err = io.Copy(tw, file)
		}
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			count++
		}
		return err
	})
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("staging directory contains no regular files")
	}
	return nil
}

type countWriter struct{ n *int64 }

func (w countWriter) Write(p []byte) (int, error) {
	*w.n += int64(len(p))
	return len(p), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
