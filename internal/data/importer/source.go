package importer

import (
	"context"
	"errors"
	"io"

	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
)

var ErrUnsupportedSource = errors.New("unsupported import source")
var ErrProvider = errors.New("import provider error")

type Request struct{ RepoID, Revision string }
type Result struct {
	ObjectRef, Format, ChecksumSHA256 string
	SizeBytes                         int64
}

type Metadata struct {
	ExternalModelID string
	Version         string
}

type File struct {
	Path string
	Size int64
}

type MetadataSource interface {
	ResolveMetadata(context.Context, Request) (Metadata, error)
}

// ContentResult is an optional streaming form of Result. Providers that can
// download content expose it without forcing the worker to buffer model files
// in memory.
type ContentResult struct {
	Result
	Body        io.ReadCloser
	ContentType string
}

type ContentSource interface {
	FetchContent(context.Context, Request) (ContentResult, error)
}

type ManifestSource interface {
	ListFiles(context.Context, Request) ([]File, error)
}

// SourceAdapter downloads metadata/content through an external provider. It
// returns a storage-neutral result; persistence and object writes stay outside.
type SourceAdapter interface {
	Fetch(context.Context, Request) (Result, error)
}

type Registry map[string]SourceAdapter

func (r Registry) Resolve(source string) (SourceAdapter, error) {
	a, ok := r[source]
	if !ok || a == nil {
		return nil, ErrUnsupportedSource
	}
	return a, nil
}

// ResolveSearcher returns the search capability for a fixed provider. Download
// adapters remain the source of truth for existing import behavior.
func (r Registry) ResolveSearcher(source string) (modelbiz.Searcher, error) {
	a, ok := r[source]
	if !ok || a == nil {
		return nil, ErrUnsupportedSource
	}
	searcher, ok := a.(modelbiz.Searcher)
	if !ok || searcher == nil {
		return nil, ErrUnsupportedSource
	}
	return searcher, nil
}

// Search dispatches an external model search to the provider selected by the
// validated source enum.
func (r Registry) Search(ctx context.Context, request modelbiz.SearchRequest) (modelbiz.SearchPage, error) {
	if err := request.Validate(); err != nil {
		return modelbiz.SearchPage{}, err
	}
	searcher, err := r.ResolveSearcher(request.Source)
	if err != nil {
		return modelbiz.SearchPage{}, err
	}
	return searcher.Search(ctx, request)
}
