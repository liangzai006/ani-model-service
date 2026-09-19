package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
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
	if e != nil {
		return modelbiz.Record{}, e
	}
	rows := []modelbiz.Record{recordRow(r, tenant)}
	e = s.withLatestVersions(ctx, tenant, rows)
	return rows[0], e
}
func (s *CatalogStore) GetModelByExternalID(ctx context.Context, tenant, externalID string) (modelbiz.Record, error) {
	r, e := s.ModelStore.GetModelByExternalID(ctx, tenant, externalID)
	if e != nil {
		return modelbiz.Record{}, e
	}
	rows := []modelbiz.Record{recordExternalRow(r)}
	e = s.withLatestVersions(ctx, tenant, rows)
	return rows[0], e
}
func (s *CatalogStore) ListModels(ctx context.Context, tenant string, options modelbiz.ListOptions) ([]modelbiz.Record, error) {
	rs, e := s.ModelStore.ListModels(ctx, tenant, options)
	if e != nil {
		return nil, e
	}
	out := make([]modelbiz.Record, len(rs))
	for i, r := range rs {
		out[i] = recordRow(GetModelRow(r), tenant)
	}
	return out, s.withLatestVersions(ctx, tenant, out)
}

// Load one latest non-deleted version per model in a single query, rather than
// performing a separate version query for each row on a page.
func (s *CatalogStore) withLatestVersions(ctx context.Context, tenant string, records []modelbiz.Record) error {
	if len(records) == 0 {
		return nil
	}
	t, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	ids := make([]pgtype.UUID, len(records))
	byID := make(map[string]*modelbiz.Record, len(records))
	for i := range records {
		id, err := uuid.Parse(records[i].ID)
		if err != nil {
			return err
		}
		ids[i] = uuidType(id)
		byID[records[i].ID] = &records[i]
	}
	versions, err := New(s.pool).LatestModelVersions(ctx, LatestModelVersionsParams{TenantID: uuidType(t), Column2: ids})
	if err != nil {
		return err
	}
	for _, row := range versions {
		v := listedVersion(ListModelVersionsRow(row))
		byID[v.ModelID].LatestVersion = &v
	}
	return nil
}
func record(r Model) modelbiz.Record {
	return modelbiz.Record{TenantID: uuid.UUID(r.TenantID.Bytes).String(), ID: uuid.UUID(r.ID.Bytes).String(), ExternalModelID: r.ModelID, Name: r.Name, DisplayName: r.DisplayName, Description: r.Description, Source: r.Source, Status: r.Status, Capabilities: r.Capabilities, TotalSizeBytes: r.TotalSizeBytes, IdempotencyKey: r.IdempotencyKey, SourceRepoID: r.SourceRepoID, ErrorMessage: r.ErrorMessage, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
}
func recordRow(r GetModelRow, tenant string) modelbiz.Record {
	return modelbiz.Record{TenantID: tenant, ID: uuid.UUID(r.ID.Bytes).String(), ExternalModelID: r.ModelID, Name: r.Name, DisplayName: r.DisplayName, Description: r.Description, Source: r.Source, Status: r.Status, Capabilities: r.Capabilities, TotalSizeBytes: r.TotalSizeBytes, IdempotencyKey: r.IdempotencyKey, SourceRepoID: r.SourceRepoID, ErrorMessage: r.ErrorMessage, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
}
func recordExternalRow(r GetModelByExternalIDRow) modelbiz.Record {
	return recordRow(GetModelRow(r), uuid.UUID(r.TenantID.Bytes).String())
}
