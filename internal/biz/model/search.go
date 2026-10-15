package model

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrInvalidSearchRequest = errors.New("invalid search request")

type SearchRequest struct {
	Source   string
	Keyword  string
	Owner    string
	Task     string
	Library  string
	Page     int
	PageSize int
	Limit    int    // Deprecated compatibility alias for PageSize.
	Cursor   string // Deprecated compatibility cursor.
}

func (r SearchRequest) Validate() error {
	if r.Source != "modelscope" && r.Source != "huggingface" {
		return ErrInvalidSearchRequest
	}
	size := r.PageSize
	if size == 0 {
		size = r.Limit
	}
	if size == 0 {
		size = 20
	}
	page := r.Page
	if page == 0 {
		page = 1
	}
	if strings.TrimSpace(r.Keyword) == "" || page < 1 || page > 100 || size < 1 || size > 100 || len(r.Cursor) > 2048 {
		return ErrInvalidSearchRequest
	}
	return nil
}

type SearchResult struct {
	Source      string
	RepoID      string
	Name        string
	DisplayName string
	Description string
	Revision    string
	SizeBytes   int64
	Tags        []string
	UpdatedAt   time.Time
}

type SearchPage struct {
	Items      []SearchResult
	NextCursor string
	HasMore    bool
	Total      int64
}

type Searcher interface {
	Search(context.Context, SearchRequest) (SearchPage, error)
}
