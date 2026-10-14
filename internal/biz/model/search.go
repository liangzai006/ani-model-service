package model

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrInvalidSearchRequest = errors.New("invalid search request")

type SearchRequest struct {
	Source  string
	Keyword string
	Owner   string
	Task    string
	Library string
	Limit   int
	Cursor  string
}

func (r SearchRequest) Validate() error {
	if r.Source != "modelscope" && r.Source != "huggingface" {
		return ErrInvalidSearchRequest
	}
	if strings.TrimSpace(r.Keyword) == "" || r.Limit < 1 || r.Limit > 50 || len(r.Cursor) > 2048 {
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
}

type Searcher interface {
	Search(context.Context, SearchRequest) (SearchPage, error)
}
