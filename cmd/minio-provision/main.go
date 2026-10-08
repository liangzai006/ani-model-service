// Command model-import is the one-shot model import entrypoint used by a
// Kubernetes Job. It downloads the requested repository with the provider CLI
// and uploads one immutable archive through a signed URL.
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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

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
	uploadURL := strings.TrimSpace(get("ANI_IMPORT_UPLOAD_URL"))
	if uploadURL == "" {
		return errors.New("ANI_IMPORT_UPLOAD_URL is required")
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
	result, err := uploadArchive(ctx, uploadURL, key, staging)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(result)
	// The Job controller consumes this stable, single-line result. It contains
	// no credentials and is safe to retain in the Job log for reconciliation.
	fmt.Printf("ANI_IMPORT_RESULT %s\n", encoded)
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
		args := []string{"download", "--revision", revision, "--local-dir", staging, repo}
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

func uploadArchive(ctx context.Context, uploadURL, objectRef, root string) (importResult, error) {
	temp, err := os.CreateTemp("", "ani-model-import-*.tar")
	if err != nil {
		return importResult{}, fmt.Errorf("create archive temporary file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	defer temp.Close()

	hasher := sha256.New()
	var size int64
	if err := archiveDirectory(root, io.MultiWriter(temp, hasher, countWriter{n: &size})); err != nil {
		return importResult{}, fmt.Errorf("archive model: %w", err)
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		return importResult{}, fmt.Errorf("rewind model archive: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, temp)
	if err != nil {
		return importResult{}, fmt.Errorf("create upload request: %w", err)
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return importResult{}, fmt.Errorf("upload model archive: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return importResult{}, fmt.Errorf("upload model archive: server returned %s", resp.Status)
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
