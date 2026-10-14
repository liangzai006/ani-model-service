package importer

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
)

type fake struct{}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func (fake) Fetch(context.Context, Request) (Result, error) { return Result{}, nil }

func TestRegistryAllowlist(t *testing.T) {
	r := Registry{"huggingface": fake{}, "modelscope": fake{}}
	if _, err := r.Resolve("huggingface"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve("builtin"); err != ErrUnsupportedSource {
		t.Fatalf("got %v", err)
	}
}

func TestHuggingFaceAdapterStreamsExplicitRepositoryFile(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/org/model/resolve/main/model.gguf" || r.URL.Query().Get("download") != "true" {
			t.Fatalf("request path = %s query = %s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.WriteString(w, "model-bytes")
	}))
	server.Listener = listener
	server.Start()
	defer server.Close()
	adapter := NewHuggingFaceAdapter(server.URL, server.Client())
	result, err := adapter.FetchContent(context.Background(), Request{RepoID: "org/model#model.gguf", Revision: "main"})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	body, err := io.ReadAll(result.Body)
	if err != nil || string(body) != "model-bytes" {
		t.Fatalf("body = %q err = %v", body, err)
	}
	if result.ObjectRef != "org/model/main/model.gguf" || result.Format != "gguf" || result.ContentType != "application/octet-stream" {
		t.Fatalf("result = %+v", result.Result)
	}
}

func TestHTTPSourceResolvesModelMetadata(t *testing.T) {
	adapter := NewHuggingFaceAdapter("https://huggingface.co", nil)
	got, err := adapter.ResolveMetadata(context.Background(), Request{RepoID: "Qwen/Qwen3-32B#model.safetensors", Revision: "v2"})
	if err != nil || got.ExternalModelID != "Qwen3-32B" || got.Version != "v2" {
		t.Fatalf("metadata=%+v error=%v", got, err)
	}
	got, err = adapter.ResolveMetadata(context.Background(), Request{RepoID: "org/model#model.gguf"})
	if err != nil || got.Version != "main" {
		t.Fatalf("default metadata=%+v error=%v", got, err)
	}
}

func TestHTTPSourceRequiresExplicitFile(t *testing.T) {
	adapter := NewHuggingFaceAdapter("https://example.invalid", nil)
	if _, err := adapter.FetchContent(context.Background(), Request{RepoID: "org/model", Revision: "main"}); !strings.Contains(err.Error(), "repo_id must be repo#file") {
		t.Fatalf("error = %v", err)
	}
}

func TestHTTPSourceResolvesRepositoryMetadataWithoutFile(t *testing.T) {
	adapter := NewHuggingFaceAdapter("https://example.invalid", nil)
	got, err := adapter.ResolveMetadata(context.Background(), Request{RepoID: "org/model", Revision: "commit"})
	if err != nil || got.ExternalModelID != "model" || got.Version != "commit" {
		t.Fatalf("metadata=%+v err=%v", got, err)
	}
}

func TestHTTPSourceListsRepositoryManifest(t *testing.T) {
	adapter := NewHuggingFaceAdapter("https://example.invalid", nil)
	adapter.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/models/org/model/revision/main" || req.URL.Query().Get("blobs") != "true" {
			t.Fatalf("manifest request=%s", req.URL.String())
		}
		body := `{"siblings":[{"rfilename":"config.json"},{"rfilename":"model.safetensors","size":12}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	files, err := adapter.ListFiles(context.Background(), Request{RepoID: "org/model"})
	if err != nil || len(files) != 2 || files[0].Path != "config.json" || files[0].Size != -1 || files[1].Size != 12 {
		t.Fatalf("files=%+v err=%v", files, err)
	}
}

func TestHuggingFaceManifestPinsRevisionAndRequestsSizes(t *testing.T) {
	adapter := NewHuggingFaceAdapter("https://example.invalid", nil)
	adapter.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.EscapedPath() != "/api/models/org/model/revision/refs%2Fpr%2F42" || req.URL.Query().Get("blobs") != "true" {
			t.Fatalf("manifest request=%s", req.URL.String())
		}
		body := `{"sha":"refs-pr-42","siblings":[{"rfilename":"model.safetensors","size":123}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	files, err := adapter.ListFiles(context.Background(), Request{RepoID: "org/model", Revision: "refs/pr/42"})
	if err != nil || len(files) != 1 || files[0].Path != "model.safetensors" || files[0].Size != 123 {
		t.Fatalf("files=%+v err=%v", files, err)
	}
}

func TestModelScopeAdapterListsFilesAndSizes(t *testing.T) {
	adapter := NewModelScopeAdapter("https://example.invalid", nil)
	adapter.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/v1/models/org/model/repo/files" || req.URL.Query().Get("Revision") != "master" || req.URL.Query().Get("Recursive") != "true" {
			t.Fatalf("manifest request=%s", req.URL.String())
		}
		body := `{"Code":200,"Success":true,"Data":{"Files":[{"Path":"config.json","Size":2,"Type":"blob"},{"Path":"weights","Size":0,"Type":"tree"},{"Path":"model.safetensors","Size":123,"Type":"blob"}]}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	files, err := adapter.ListFiles(context.Background(), Request{RepoID: "org/model"})
	if err != nil || len(files) != 2 || files[0].Path != "config.json" || files[0].Size != 2 || files[1].Size != 123 {
		t.Fatalf("files=%+v err=%v", files, err)
	}
	metadata, err := adapter.ResolveMetadata(context.Background(), Request{RepoID: "org/model"})
	if err != nil || metadata.Version != "master" {
		t.Fatalf("metadata=%+v err=%v", metadata, err)
	}
}

func TestRegistrySearchModelScope(t *testing.T) {
	adapter := NewModelScopeAdapter("https://example.invalid", nil)
	adapter.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		query := req.URL.Query()
		if req.URL.Path != "/openapi/v1/models" || query.Get("search") != "Qwen" || query.Get("page_size") != "2" || query.Get("owner") != "Qwen" || query.Get("filter.task") != "text-generation" || query.Get("filter.library") != "transformers" {
			t.Fatalf("search request=%s", req.URL.String())
		}
		body := `{"data":{"models":[{"id":"Qwen/Qwen3","name":"Qwen3","display_name":"Qwen 3","description":"A model","tags":["llm","qwen"],"last_modified":"2025-01-02T03:04:05Z"}],"total_count":1,"page_number":1,"page_size":2}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	page, err := Registry{"modelscope": adapter}.Search(context.Background(), modelbiz.SearchRequest{Source: "modelscope", Keyword: "Qwen", Owner: "Qwen", Task: "text-generation", Library: "transformers", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items=%+v", page.Items)
	}
	got := page.Items[0]
	if got.Source != "modelscope" || got.RepoID != "Qwen/Qwen3" || got.Name != "Qwen3" || got.DisplayName != "Qwen 3" || got.Description != "A model" || got.Revision != "master" || got.SizeBytes != 0 || len(got.Tags) != 2 || !got.UpdatedAt.Equal(time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("result=%+v", got)
	}
	if page.HasMore || page.NextCursor != "" {
		t.Fatalf("unexpected pagination=%+v", page)
	}
}

func TestRegistrySearchModelScopePagination(t *testing.T) {
	adapter := NewModelScopeAdapter("https://example.invalid", nil)
	adapter.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		query := req.URL.Query()
		if query.Get("page_number") != "2" || query.Get("page_size") != "2" {
			t.Fatalf("search request=%s", req.URL.String())
		}
		body := `{"data":{"models":[{"id":"org/model-3"},{"id":"org/model-4"}],"total_count":5,"page_number":2,"page_size":2}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	page, err := Registry{"modelscope": adapter}.Search(context.Background(), modelbiz.SearchRequest{Source: "modelscope", Keyword: "model", Limit: 2, Cursor: encodeSearchCursor(2)})
	if err != nil || len(page.Items) != 2 || !page.HasMore {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	nextPage, err := decodeSearchCursor(page.NextCursor)
	if err != nil || nextPage != 3 {
		t.Fatalf("next cursor=%q page=%d err=%v", page.NextCursor, nextPage, err)
	}
}

func TestRegistrySearchHuggingFace(t *testing.T) {
	adapter := NewHuggingFaceAdapter("https://example.invalid", nil)
	adapter.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		query := req.URL.Query()
		if req.URL.Path != "/api/models" || query.Get("search") != "Qwen" || query.Get("limit") != "2" || query.Get("author") != "Qwen" || query.Get("pipeline_tag") != "text-generation" || query.Get("library") != "transformers" {
			t.Fatalf("search request=%s", req.URL.String())
		}
		body := `[{"id":"Qwen/Qwen3","tags":["llm","qwen"],"pipeline_tag":"text-generation","lastModified":"2025-01-02T03:04:05Z","sha":"abc123"}]`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	page, err := Registry{"huggingface": adapter}.Search(context.Background(), modelbiz.SearchRequest{Source: "huggingface", Keyword: "Qwen", Owner: "Qwen", Task: "text-generation", Library: "transformers", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items=%+v", page.Items)
	}
	got := page.Items[0]
	if got.Source != "huggingface" || got.RepoID != "Qwen/Qwen3" || got.Name != "Qwen3" || got.DisplayName != "Qwen3" || got.Revision != "abc123" || got.SizeBytes != 0 || len(got.Tags) != 3 || !got.UpdatedAt.Equal(time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("result=%+v", got)
	}
	if got.Tags[2] != "text-generation" {
		t.Fatalf("tags=%v", got.Tags)
	}
}

func TestRegistrySearchUsesOpaqueProviderPageCursor(t *testing.T) {
	adapter := NewHuggingFaceAdapter("https://example.invalid", nil)
	adapter.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("offset") != "2" {
			t.Fatalf("search request=%s", req.URL.String())
		}
		body := `[{"id":"org/model","lastModified":"2025-01-02T03:04:05Z"}]`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	cursor := encodeSearchCursor(2)
	page, err := Registry{"huggingface": adapter}.Search(context.Background(), modelbiz.SearchRequest{Source: "huggingface", Keyword: "Qwen", Limit: 2, Cursor: cursor})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

func TestRegistrySearchRejectsUpstreamFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		code int
	}{
		{name: "non-2xx", body: `{"error":"rate limited"}`, code: http.StatusTooManyRequests},
		{name: "malformed json", body: `{`, code: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := NewHuggingFaceAdapter("https://example.invalid", nil)
			adapter.Client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.code, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			_, err := Registry{"huggingface": adapter}.Search(context.Background(), modelbiz.SearchRequest{Source: "huggingface", Keyword: "Qwen", Limit: 2})
			if !errors.Is(err, ErrProvider) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestRegistrySearchEscapesQueryParameters(t *testing.T) {
	adapter := NewHuggingFaceAdapter("https://example.invalid", nil)
	adapter.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("search") != "Qwen & 3" || req.URL.RawQuery == "" {
			t.Fatalf("search request=%s", req.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`[]`)), Header: make(http.Header)}, nil
	})}
	_, err := Registry{"huggingface": adapter}.Search(context.Background(), modelbiz.SearchRequest{Source: "huggingface", Keyword: "Qwen & 3", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
}
