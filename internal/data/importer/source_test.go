package importer

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
		if req.URL.Path != "/api/models/org/model" || req.URL.Query().Get("revision") != "main" {
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
