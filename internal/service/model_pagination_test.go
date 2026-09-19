package service

import (
	"context"
	"strings"
	"testing"
	"time"

	kratoserrors "github.com/go-kratos/kratos/v3/errors"

	modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
	"github.com/liangzai006/ani-model-service/internal/biz/model"
)

type paginationCatalog struct {
	catalogFake
	options model.ListOptions
	calls   int
}

func (c *paginationCatalog) ListModels(_ context.Context, _ string, options model.ListOptions) ([]model.Record, error) {
	c.options, c.calls = options, c.calls+1
	created := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	if options.BeforeID != "" {
		return []model.Record{{ID: "11111111-1111-4111-8111-111111111111", CreatedAt: created}}, nil
	}
	return []model.Record{
		{ID: "11111111-1111-4111-8111-111111111113", Name: "model-c", CreatedAt: created},
		{ID: "11111111-1111-4111-8111-111111111112", Name: "model-b", CreatedAt: created},
		{ID: "11111111-1111-4111-8111-111111111111", Name: "model-a", CreatedAt: created},
	}, nil
}

func TestListModelsReturnsPageAndContinuation(t *testing.T) {
	catalog := &paginationCatalog{}
	server := NewModelCatalogService(nil, catalog)
	request := &modelv1.ListModelsRequest{Status: "ready", Source: "huggingface", Keyword: "smol", Capability: "chat", Page: &modelv1.CursorPageRequest{Limit: 2}}
	result, err := server.ListModels(referenceContext("tenant-a"), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.GetModels()) != 2 || !result.GetMeta().GetHasMore() || result.GetMeta().GetNextCursor() == "" {
		t.Fatalf("expected two models and continuation, got %v", result)
	}
	if catalog.options.Limit != 3 || catalog.options.Status != "ready" || catalog.options.Keyword != "smol" || catalog.options.Source != "huggingface" || catalog.options.Capability != "chat" {
		t.Fatalf("filters not forwarded: %+v", catalog.options)
	}
	request.Page.Cursor = result.Meta.NextCursor
	last, err := server.ListModels(referenceContext("tenant-a"), request)
	if err != nil || len(last.GetModels()) != 1 || last.GetMeta().GetHasMore() || last.GetMeta().GetNextCursor() != "" {
		t.Fatalf("last page=%v err=%v", last, err)
	}
	if catalog.options.BeforeID != result.Models[1].Id || catalog.options.BeforeCreatedAt.IsZero() {
		t.Fatalf("wrong boundary: %+v", catalog.options)
	}
}

func TestListModelsRejectsWrongScopeAndInvalidPageBeforeStore(t *testing.T) {
	catalog := &paginationCatalog{}
	server := NewModelCatalogService(nil, catalog)
	scope := cursorScope("tenant-a", "models", "", "", "", "")
	cursor := nextCursor(scope, time.Now().UTC(), referenceModelUUID)
	for _, tc := range []struct {
		name, tenant, keyword, source, capability, status, cursor string
		limit                                                     int32
	}{
		{name: "tenant", tenant: "tenant-b", cursor: cursor},
		{name: "keyword", tenant: "tenant-a", keyword: "x", cursor: cursor},
		{name: "source", tenant: "tenant-a", source: "upload", cursor: cursor},
		{name: "capability", tenant: "tenant-a", capability: "embedding", cursor: cursor},
		{name: "status", tenant: "tenant-a", status: "ready", cursor: cursor},
		{name: "malformed", tenant: "tenant-a", cursor: "bad!"},
		{name: "oversized", tenant: "tenant-a", cursor: strings.Repeat("a", 2049)},
		{name: "negative limit", tenant: "tenant-a", limit: -1},
		{name: "excessive limit", tenant: "tenant-a", limit: 1001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := server.ListModels(referenceContext(tc.tenant), &modelv1.ListModelsRequest{Keyword: tc.keyword, Source: tc.source, Capability: tc.capability, Status: tc.status, Page: &modelv1.CursorPageRequest{Limit: tc.limit, Cursor: tc.cursor}})
			if kratoserrors.Code(err) != 400 || catalog.calls != 0 {
				t.Fatalf("error=%v store calls=%d", err, catalog.calls)
			}
		})
	}
}

func TestModelProtoMetadata(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	v := model.Version{ID: referenceModelUUID, Version: "v2", SizeBytes: 123, IsEncrypted: true, EncryptAlgo: "AES-256", EncryptHint: "key-a", CreatedAt: now, ArtifactReference: "tenant/model/v2/model.tar", ArtifactSHA256: strings.Repeat("a", 64)}
	m := toProtoModel(model.Record{CreatedAt: now, UpdatedAt: now, SourceRepoID: "org/model", ErrorMessage: "download failed", LatestVersion: &v})
	if m.SourceRepoId != "org/model" || m.ErrorMessage != "download failed" || !m.GetCreatedAt().AsTime().Equal(now) || !m.GetUpdatedAt().AsTime().Equal(now) || len(m.Versions) != 1 {
		t.Fatalf("missing model metadata: %v", m)
	}
	got := m.Versions[0]
	if got.SizeBytes != 123 || !got.IsEncrypted || got.EncryptAlgo != "AES-256" || got.EncryptHint != "key-a" || got.StoragePath != v.ArtifactReference || !got.GetCreatedAt().AsTime().Equal(now) {
		t.Fatalf("missing version metadata: %v", got)
	}
}

type paginationVersions struct {
	versionCatalogFake
	options model.ListOptions
	calls   int
}

func (v *paginationVersions) ListVersions(_ context.Context, tenant, modelID string, options model.ListOptions) ([]model.Version, error) {
	v.options, v.calls = options, v.calls+1
	created := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	if options.BeforeID != "" {
		return []model.Version{{ID: referenceModelUUID, TenantID: tenant, ModelID: modelID, Version: "v1", CreatedAt: created}}, nil
	}
	return []model.Version{
		{ID: "11111111-1111-4111-8111-111111111113", TenantID: tenant, ModelID: modelID, Version: "v3", CreatedAt: created},
		{ID: "11111111-1111-4111-8111-111111111112", TenantID: tenant, ModelID: modelID, Version: "v2", CreatedAt: created},
		{ID: referenceModelUUID, TenantID: tenant, ModelID: modelID, Version: "v1", CreatedAt: created},
	}, nil
}

func TestListVersionsContinuationAndModelScope(t *testing.T) {
	versions := &paginationVersions{}
	server := NewModelServiceWithDependencies(versions, versions, &catalogFake{}, nil, nil)
	request := &modelv1.ListModelVersionsRequest{ModelId: referenceModelUUID, Page: &modelv1.CursorPageRequest{Limit: 2}}
	first, err := server.ListModelVersions(referenceContext("tenant-a"), request)
	if err != nil || len(first.GetVersions()) != 2 || !first.GetMeta().GetHasMore() || first.GetMeta().GetNextCursor() == "" {
		t.Fatalf("first page=%v err=%v", first, err)
	}
	request.Page.Cursor = first.Meta.NextCursor
	last, err := server.ListModelVersions(referenceContext("tenant-a"), request)
	if err != nil || len(last.GetVersions()) != 1 || last.GetMeta().GetHasMore() || last.GetMeta().GetNextCursor() != "" || versions.options.BeforeID != first.Versions[1].Id {
		t.Fatalf("last page=%v err=%v options=%+v", last, err, versions.options)
	}
	request.ModelId = "22222222-2222-4222-8222-222222222222"
	_, err = server.ListModelVersions(referenceContext("tenant-a"), request)
	if kratoserrors.Code(err) != 400 || versions.calls != 2 {
		t.Fatalf("cursor accepted for other model: %v calls=%d", err, versions.calls)
	}
	request.ModelId = referenceModelUUID
	_, err = server.ListModelVersions(referenceContext("tenant-b"), request)
	if kratoserrors.Code(err) != 400 || versions.calls != 2 {
		t.Fatalf("cursor accepted for other tenant: %v calls=%d", err, versions.calls)
	}
}

func TestEmptyListsReturnTerminalPage(t *testing.T) {
	server := NewModelServiceWithDependencies(nil, &versionCatalogFake{}, &catalogFake{}, nil, nil)
	models, err := server.ListModels(referenceContext("tenant-a"), &modelv1.ListModelsRequest{})
	if err != nil || len(models.GetModels()) != 0 || models.Meta == nil || models.Meta.HasMore || models.Meta.NextCursor != "" {
		t.Fatalf("empty models=%v err=%v", models, err)
	}
	versions, err := server.ListModelVersions(referenceContext("tenant-a"), &modelv1.ListModelVersionsRequest{ModelId: referenceModelUUID})
	if err != nil || len(versions.GetVersions()) != 0 || versions.Meta == nil || versions.Meta.HasMore || versions.Meta.NextCursor != "" {
		t.Fatalf("empty versions=%v err=%v", versions, err)
	}
}
