package postgres

import (
	"context"

	"github.com/google/uuid"
	modelbiz "github.com/zhangzhe-ctrl/ani-model-service/internal/biz/model"
)

type CatalogStore struct{ *ModelStore }

func NewCatalogStore(s *ModelStore) *CatalogStore { return &CatalogStore{ModelStore: s} }
func (s *CatalogStore) CreateModel(ctx context.Context, tenant, id, name, display, description, source string, caps []byte, idempotencyKey string) (modelbiz.Record, error) {
	r, e := s.ModelStore.CreateModel(ctx, tenant, id, name, display, description, source, caps, idempotencyKey)
	return record(r), e
}
func (s *CatalogStore) CreateModelWithExternalID(ctx context.Context, tenant, id, externalID, name, display, description, source string, caps []byte, idempotencyKey string) (modelbiz.Record, error) {
	r, e := s.ModelStore.CreateModelWithExternalID(ctx, tenant, id, externalID, name, display, description, source, caps, idempotencyKey)
	return record(r), e
}
func (s *CatalogStore) GetModel(ctx context.Context, tenant, id string) (modelbiz.Record, error) {
	r, e := s.ModelStore.GetModel(ctx, tenant, id)
	return recordRow(r, tenant), e
}
func (s *CatalogStore) GetModelByExternalID(ctx context.Context, tenant, externalID string) (modelbiz.Record, error) {
	r, e := s.ModelStore.GetModelByExternalID(ctx, tenant, externalID)
	return recordExternalRow(r), e
}
func (s *CatalogStore) ListModels(ctx context.Context, tenant, status string, limit int32) ([]modelbiz.Record, error) {
	rs, e := s.ModelStore.ListModels(ctx, tenant, status, limit)
	out := make([]modelbiz.Record, len(rs))
	for i, r := range rs {
		out[i] = modelbiz.Record{TenantID: tenant, ID: uuid.UUID(r.ID.Bytes).String(), ExternalModelID: r.ModelID, Name: r.Name, DisplayName: r.DisplayName, Description: r.Description, Source: r.Source, Status: r.Status, Capabilities: r.Capabilities, TotalSizeBytes: r.TotalSizeBytes}
	}
	return out, e
}
func (s *CatalogStore) SoftDeleteModel(ctx context.Context, tenant, id string) error {
	return s.ModelStore.SoftDeleteModel(ctx, tenant, id)
}
func record(r Model) modelbiz.Record {
	return modelbiz.Record{TenantID: uuid.UUID(r.TenantID.Bytes).String(), ID: uuid.UUID(r.ID.Bytes).String(), ExternalModelID: r.ModelID, Name: r.Name, DisplayName: r.DisplayName, Description: r.Description, Source: r.Source, Status: r.Status, Capabilities: r.Capabilities, TotalSizeBytes: r.TotalSizeBytes, IdempotencyKey: r.IdempotencyKey}
}
func recordRow(r GetModelRow, tenant string) modelbiz.Record {
	return modelbiz.Record{TenantID: tenant, ID: uuid.UUID(r.ID.Bytes).String(), ExternalModelID: r.ModelID, Name: r.Name, DisplayName: r.DisplayName, Description: r.Description, Source: r.Source, Status: r.Status, Capabilities: r.Capabilities, TotalSizeBytes: r.TotalSizeBytes, IdempotencyKey: r.IdempotencyKey}
}
func recordExternalRow(r GetModelByExternalIDRow) modelbiz.Record {
	return modelbiz.Record{TenantID: uuid.UUID(r.TenantID.Bytes).String(), ID: uuid.UUID(r.ID.Bytes).String(), ExternalModelID: r.ModelID, Name: r.Name, DisplayName: r.DisplayName, Description: r.Description, Source: r.Source, Status: r.Status, Capabilities: r.Capabilities, TotalSizeBytes: r.TotalSizeBytes, IdempotencyKey: r.IdempotencyKey}
}
