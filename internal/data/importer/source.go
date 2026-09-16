package importer

import (
	"context"
	"errors"
	"io"
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
