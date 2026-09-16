package service

import (
	"context"
	"errors"
	"testing"

	kratoserrors "github.com/go-kratos/kratos/v3/errors"
	"github.com/jackc/pgx/v5"
	modelv1 "github.com/zhangzhe-ctrl/ani-model-service/api/model/v1"
	"github.com/zhangzhe-ctrl/ani-model-service/internal/biz/model"
	"github.com/zhangzhe-ctrl/ani-model-service/internal/identity"
)

const referenceModelUUID = "11111111-1111-4111-8111-111111111111"

type referenceCatalog struct {
	catalogFake
	err       error
	deletedID string
}

func (c *referenceCatalog) GetModel(_ context.Context, tenant, id string) (model.Record, error) {
	if c.err != nil {
		return model.Record{}, c.err
	}
	if tenant != "tenant-a" || id != referenceModelUUID {
		return model.Record{}, pgx.ErrNoRows
	}
	return model.Record{TenantID: tenant, ID: id, ExternalModelID: "Qwen3-32B", Name: "qwen3-32b"}, nil
}
func (c *referenceCatalog) GetModelByExternalID(ctx context.Context, tenant, id string) (model.Record, error) {
	if id != "Qwen3-32B" {
		return model.Record{}, pgx.ErrNoRows
	}
	return c.GetModel(ctx, tenant, referenceModelUUID)
}
func (c *referenceCatalog) SoftDeleteModel(_ context.Context, tenant, id string) error {
	if tenant != "tenant-a" || id != referenceModelUUID {
		return pgx.ErrNoRows
	}
	c.deletedID = id
	return nil
}

type referenceVersionCatalog struct {
	versionCatalogFake
	listedID string
}

func (c *referenceVersionCatalog) ListVersions(_ context.Context, tenant, id string, _ int32) ([]model.Version, error) {
	c.listedID = id
	return []model.Version{{TenantID: tenant, ID: "version-uuid", ModelID: id, Version: "v2", Format: "gguf", Status: "pending"}}, nil
}

type referenceCheckFunc func(context.Context, string, string) (bool, error)

func (f referenceCheckFunc) HasActiveReferences(ctx context.Context, tenant, id string) (bool, error) {
	return f(ctx, tenant, id)
}

func referenceContext(tenant string) context.Context {
	return identity.WithPrincipal(context.Background(), identity.Principal{TenantID: tenant, Actor: "actor", Workload: "gateway"})
}

func TestModelCRUDResolvesExternalID(t *testing.T) {
	for _, selector := range []string{"Qwen3-32B", referenceModelUUID} {
		t.Run(selector, func(t *testing.T) {
			catalog, versions := &referenceCatalog{}, &referenceVersionCatalog{}
			s := NewModelServiceWithDependencies(versions, versions, catalog, nil, nil)
			ctx := referenceContext("tenant-a")
			got, err := s.GetModel(ctx, &modelv1.GetModelRequest{ModelId: selector})
			if err != nil || got.GetId() != referenceModelUUID || got.GetModelId() != "Qwen3-32B" {
				t.Fatalf("get=%v error=%v", got, err)
			}
			created, err := s.CreateModelVersion(ctx, &modelv1.CreateModelVersionRequest{ModelId: selector, Version: "v2", Format: "gguf"})
			if err != nil || versions.created.ModelID != referenceModelUUID || created.GetModelId() != "Qwen3-32B" {
				t.Fatalf("create=%v stored=%+v error=%v", created, versions.created, err)
			}
			listed, err := s.ListModelVersions(ctx, &modelv1.ListModelVersionsRequest{ModelId: selector})
			if err != nil || versions.listedID != referenceModelUUID || len(listed.GetVersions()) != 1 || listed.GetVersions()[0].GetModelId() != "Qwen3-32B" {
				t.Fatalf("list=%v internal_id=%s error=%v", listed, versions.listedID, err)
			}
		})
	}
}

func TestModelReferenceResolutionRejectsTenantMismatchAndMissingModel(t *testing.T) {
	for _, tc := range []struct {
		name, principal, requested, selector string
		code                                 int
	}{
		{"mismatch", "tenant-a", "tenant-b", "Qwen3-32B", 403},
		{"other tenant", "tenant-b", "tenant-b", "Qwen3-32B", 404},
		{"case sensitive", "tenant-a", "tenant-a", "qwen3-32b", 404},
		{"empty selector", "tenant-a", "tenant-a", "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			versions := &referenceVersionCatalog{}
			s := NewModelServiceWithDependencies(versions, versions, &referenceCatalog{}, nil, nil)
			ctx := referenceContext(tc.principal)
			_, err := s.GetModel(ctx, &modelv1.GetModelRequest{TenantId: tc.requested, ModelId: tc.selector})
			if kratoserrors.Code(err) != tc.code {
				t.Fatalf("get code=%d error=%v", kratoserrors.Code(err), err)
			}
			_, err = s.CreateModelVersion(ctx, &modelv1.CreateModelVersionRequest{TenantId: tc.requested, ModelId: tc.selector, Version: "v2", Format: "gguf"})
			if kratoserrors.Code(err) != tc.code || versions.created.ID != "" {
				t.Fatalf("create code=%d error=%v", kratoserrors.Code(err), err)
			}
			_, err = s.ListModelVersions(ctx, &modelv1.ListModelVersionsRequest{TenantId: tc.requested, ModelId: tc.selector})
			if kratoserrors.Code(err) != tc.code || versions.listedID != "" {
				t.Fatalf("list code=%d error=%v", kratoserrors.Code(err), err)
			}
		})
	}
}

func TestDeleteModelChecksBothReferenceIdentifiers(t *testing.T) {
	for _, blockedID := range []string{"", "Qwen3-32B", referenceModelUUID} {
		t.Run("blocked="+blockedID, func(t *testing.T) {
			catalog := &referenceCatalog{}
			s := NewModelCatalogService(nil, catalog)
			checked := map[string]bool{}
			s.SetInferenceReferenceChecker(referenceCheckFunc(func(_ context.Context, tenant, id string) (bool, error) {
				if tenant != "tenant-a" {
					t.Fatalf("wrong tenant %s", tenant)
				}
				checked[id] = true
				return id == blockedID, nil
			}))
			_, err := s.DeleteModel(referenceContext("tenant-a"), &modelv1.DeleteModelRequest{ModelId: "Qwen3-32B"})
			if blockedID != "" {
				if kratoserrors.Code(err) != 409 || catalog.deletedID != "" {
					t.Fatalf("delete error=%v deleted=%s", err, catalog.deletedID)
				}
			} else if err != nil || catalog.deletedID != referenceModelUUID || !checked[referenceModelUUID] || !checked["Qwen3-32B"] {
				t.Fatalf("delete error=%v deleted=%s checked=%v", err, catalog.deletedID, checked)
			}
		})
	}
}

func TestGetModelProviderErrorIsNotNotFound(t *testing.T) {
	s := NewModelCatalogService(nil, &referenceCatalog{err: errors.New("database unavailable")})
	_, err := s.GetModel(referenceContext("tenant-a"), &modelv1.GetModelRequest{ModelId: "Qwen3-32B"})
	if kratoserrors.Code(err) != 500 {
		t.Fatalf("code=%d error=%v", kratoserrors.Code(err), err)
	}
}

func TestImportModelResolvesExternalModelID(t *testing.T) {
	work := &importWorkFake{}
	catalog := &referenceCatalog{}
	s := NewModelServiceWithDependencies(nil, nil, catalog, nil, work)
	_, err := s.ImportModel(referenceContext("tenant-a"), &modelv1.ImportModelRequest{Source: "huggingface", RepoId: "Qwen/Qwen3-32B#model.gguf", Revision: "v2", IdempotencyKey: "import-external", ModelId: "Qwen3-32B"})
	if err != nil || work.got.ModelID != referenceModelUUID {
		t.Fatalf("task=%+v error=%v", work.got, err)
	}
}
