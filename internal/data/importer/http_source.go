package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// HTTPSourceAdapter downloads one explicitly selected repository file. The
// file is supplied as repo_id#file in the ImportModel request; no model or
// filename is hard-coded by the service.
type HTTPSourceAdapter struct {
	BaseURL     string
	Client      *http.Client
	Resolve     func(repoID, revision, file string) string
	ManifestURL func(repoID, revision string) string
}

func (a *HTTPSourceAdapter) ResolveMetadata(_ context.Context, req Request) (Metadata, error) {
	repo, err := splitRepo(req.RepoID)
	if err != nil {
		return Metadata{}, err
	}
	version := req.Revision
	if version == "" {
		version = "main"
	}
	modelID := path.Base(repo)
	if modelID == "." || modelID == "/" || modelID == "" {
		return Metadata{}, fmt.Errorf("%w: model id is empty", ErrProvider)
	}
	return Metadata{ExternalModelID: modelID, Version: version}, nil
}

func NewHuggingFaceAdapter(baseURL string, client *http.Client) *HTTPSourceAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://huggingface.co"
	}
	return &HTTPSourceAdapter{BaseURL: strings.TrimRight(baseURL, "/"), Client: client, Resolve: func(repo, revision, file string) string {
		return strings.TrimRight(baseURL, "/") + "/" + escapePath(repo) + "/resolve/" + url.PathEscape(revision) + "/" + escapePath(file) + "?download=true"
	}, ManifestURL: func(repo, revision string) string {
		return strings.TrimRight(baseURL, "/") + "/api/models/" + escapePath(repo) + "?revision=" + url.QueryEscape(revision)
	}}
}

func NewModelScopeAdapter(baseURL string, client *http.Client) *HTTPSourceAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://www.modelscope.cn"
	}
	return &HTTPSourceAdapter{BaseURL: strings.TrimRight(baseURL, "/"), Client: client, Resolve: func(repo, revision, file string) string {
		return strings.TrimRight(baseURL, "/") + "/models/" + escapePath(repo) + "/resolve/" + url.PathEscape(revision) + "/" + escapePath(file)
	}, ManifestURL: func(repo, revision string) string {
		return strings.TrimRight(baseURL, "/") + "/api/v1/models/" + escapePath(repo) + "/repo/files?Revision=" + url.QueryEscape(revision)
	}}
}

func (a *HTTPSourceAdapter) Fetch(ctx context.Context, req Request) (Result, error) {
	content, err := a.FetchContent(ctx, req)
	if err != nil {
		return Result{}, err
	}
	if content.Body != nil {
		content.Body.Close()
	}
	return content.Result, nil
}

func (a *HTTPSourceAdapter) FetchContent(ctx context.Context, req Request) (ContentResult, error) {
	repo, file, err := splitRepoFile(req.RepoID)
	if err != nil {
		return ContentResult{}, err
	}
	revision := req.Revision
	if revision == "" {
		revision = "main"
	}
	if a.Resolve == nil {
		return ContentResult{}, fmt.Errorf("%w: resolver is not configured", ErrProvider)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, a.Resolve(repo, revision, file), nil)
	if err != nil {
		return ContentResult{}, fmt.Errorf("%w: create download request: %v", ErrProvider, err)
	}
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return ContentResult{}, fmt.Errorf("%w: download: %v", ErrProvider, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		resp.Body.Close()
		return ContentResult{}, fmt.Errorf("%w: download returned status %d", ErrProvider, resp.StatusCode)
	}
	return ContentResult{Result: Result{
		ObjectRef: repo + "/" + revision + "/" + file,
		Format:    formatForFile(file),
		SizeBytes: resp.ContentLength,
	}, Body: resp.Body, ContentType: resp.Header.Get("Content-Type")}, nil
}

func (a *HTTPSourceAdapter) ListFiles(ctx context.Context, req Request) ([]File, error) {
	repo, err := splitRepo(req.RepoID)
	if err != nil {
		return nil, err
	}
	revision := req.Revision
	if revision == "" {
		revision = "main"
	}
	if a.ManifestURL == nil {
		return nil, fmt.Errorf("%w: manifest resolver is not configured", ErrProvider)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, a.ManifestURL(repo, revision), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: create manifest request: %v", ErrProvider, err)
	}
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: manifest request: %v", ErrProvider, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: manifest returned status %d", ErrProvider, resp.StatusCode)
	}
	var payload struct {
		Siblings []struct {
			Path string `json:"rfilename"`
			Size int64  `json:"size"`
		} `json:"siblings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: decode manifest: %v", ErrProvider, err)
	}
	files := make([]File, 0, len(payload.Siblings))
	for _, sibling := range payload.Siblings {
		if strings.TrimSpace(sibling.Path) != "" {
			if sibling.Size == 0 {
				sibling.Size = -1
			}
			files = append(files, File{Path: sibling.Path, Size: sibling.Size})
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: manifest contains no files", ErrProvider)
	}
	return files, nil
}

func splitRepoFile(repoID string) (string, string, error) {
	parts := strings.SplitN(repoID, "#", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("%w: repo_id must be repo#file", ErrProvider)
	}
	if strings.Contains(parts[0], "..") || strings.Contains(parts[1], "..") {
		return "", "", fmt.Errorf("%w: invalid repository file", ErrProvider)
	}
	return strings.Trim(parts[0], "/"), strings.Trim(parts[1], "/"), nil
}

func splitRepo(repoID string) (string, error) {
	value := strings.TrimSpace(repoID)
	if strings.Contains(value, "#") {
		parts := strings.SplitN(value, "#", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
			return "", fmt.Errorf("%w: invalid repository reference", ErrProvider)
		}
		value = parts[0]
	}
	if value == "" || strings.Contains(value, "..") {
		return "", fmt.Errorf("%w: invalid repository", ErrProvider)
	}
	return strings.Trim(value, "/"), nil
}

func escapePath(value string) string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return path.Join(parts...)
}

func formatForFile(file string) string {
	switch {
	case strings.HasSuffix(file, ".gguf"):
		return "gguf"
	case strings.HasSuffix(file, ".safetensors"):
		return "safetensors"
	default:
		return "pytorch"
	}
}
