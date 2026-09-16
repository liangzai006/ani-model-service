package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-model-service/internal/biz/idempotency"
	modelbiz "github.com/zhangzhe-ctrl/ani-model-service/internal/biz/model"
)

type ModelStore struct{ pool *pgxpool.Pool }

func NewModelStore(pool *pgxpool.Pool) *ModelStore { return &ModelStore{pool: pool} }

func (s *ModelStore) CreateModel(ctx context.Context, tenantID, modelID, name, displayName, description, source string, capabilities []byte, idempotencyKey string) (Model, error) {
	return s.createModel(ctx, tenantID, modelID, name, displayName, description, source, capabilities, idempotencyKey)
}

// CreateModelWithExternalID stores a caller-facing stable model_id separately
// from the display/name field while preserving the legacy Catalog signature.
func (s *ModelStore) CreateModelWithExternalID(ctx context.Context, tenantID, internalID, externalID, name, displayName, description, source string, capabilities []byte, idempotencyKey string) (Model, error) {
	tenant, id, err := parseModelIDs(tenantID, internalID)
	if err != nil {
		return Model{}, err
	}
	return s.createModelParsed(ctx, tenant, id, externalID, name, displayName, description, source, capabilities, idempotencyKey)
}

func (s *ModelStore) createModel(ctx context.Context, tenantID, modelID, name, displayName, description, source string, capabilities []byte, idempotencyKey string) (Model, error) {
	tenant, id, err := parseModelIDs(tenantID, modelID)
	if err != nil {
		return Model{}, err
	}
	return s.createModelParsed(ctx, tenant, id, name, name, displayName, description, source, capabilities, idempotencyKey)
}

func (s *ModelStore) createModelParsed(ctx context.Context, tenant uuid.UUID, id uuid.UUID, externalID, name, displayName, description, source string, capabilities []byte, idempotencyKey string) (Model, error) {
	fingerprintBytes, _ := json.Marshal(struct {
		ExternalModelID, Name, DisplayName, Description, Source string
		Capabilities                                            []byte
	}{externalID, name, displayName, description, source, capabilities})
	fingerprint := idempotency.Fingerprint(fingerprintBytes)
	q := New(s.pool)
	if idempotencyKey != "" {
		if existing, lookupErr := q.GetModelByIdempotency(ctx, GetModelByIdempotencyParams{TenantID: uuidType(tenant), IdempotencyKey: idempotencyKey}); lookupErr == nil {
			if existing.RequestFingerprint != fingerprint {
				return Model{}, &pgconn.PgError{Code: "23505", Message: "idempotency key payload conflict"}
			}
			return existing, nil
		} else if !errors.Is(lookupErr, pgx.ErrNoRows) {
			return Model{}, lookupErr
		}
	}
	return q.CreateModel(ctx, CreateModelParams{TenantID: uuidType(tenant), ID: uuidType(id), ModelID: externalID, Name: name, DisplayName: displayName, Description: description, Source: source, Capabilities: capabilities, IdempotencyKey: idempotencyKey, RequestFingerprint: fingerprint})
}

func (s *ModelStore) GetModel(ctx context.Context, tenantID, modelID string) (GetModelRow, error) {
	tenant, id, err := parseModelIDs(tenantID, modelID)
	if err != nil {
		return GetModelRow{}, err
	}
	return New(s.pool).GetModel(ctx, GetModelParams{TenantID: uuidType(tenant), ID: uuidType(id)})
}

func (s *ModelStore) GetModelByExternalID(ctx context.Context, tenantID, externalID string) (GetModelByExternalIDRow, error) {
	tenant, err := uuid.Parse(tenantID)
	if err != nil {
		return GetModelByExternalIDRow{}, fmt.Errorf("tenant_id: %w", err)
	}
	return New(s.pool).GetModelByExternalID(ctx, GetModelByExternalIDParams{TenantID: uuidType(tenant), ModelID: externalID})
}

func (s *ModelStore) ListModels(ctx context.Context, tenantID, status string, limit int32) ([]ListModelsRow, error) {
	tenant, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant_id: %w", err)
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	return New(s.pool).ListModels(ctx, ListModelsParams{TenantID: uuidType(tenant), Column2: status, Limit: limit})
}

func (s *ModelStore) SoftDeleteModel(ctx context.Context, tenantID, modelID string) error {
	tenant, id, err := parseModelIDs(tenantID, modelID)
	if err != nil {
		return err
	}
	n, err := New(s.pool).SoftDeleteModel(ctx, SoftDeleteModelParams{TenantID: uuidType(tenant), ID: uuidType(id)})
	if err != nil {
		return err
	}
	if n != 1 {
		return pgx.ErrNoRows
	}
	return nil
}

func parseModelIDs(tenantID, modelID string) (uuid.UUID, uuid.UUID, error) {
	t, err := uuid.Parse(tenantID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("tenant_id: %w", err)
	}
	id, err := uuid.Parse(modelID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("model_id: %w", err)
	}
	return t, id, nil
}

func (s *ModelStore) GetVersion(ctx context.Context, tenantID, versionID string) (modelbiz.Version, error) {
	tenant, err := uuid.Parse(tenantID)
	if err != nil {
		return modelbiz.Version{}, fmt.Errorf("tenant_id: %w", err)
	}
	version, err := uuid.Parse(versionID)
	if err != nil {
		return modelbiz.Version{}, fmt.Errorf("model_version_id: %w", err)
	}
	row, err := New(s.pool).GetReadyModelVersion(ctx, GetReadyModelVersionParams{TenantID: pgtype.UUID{Bytes: tenant, Valid: true}, ID: pgtype.UUID{Bytes: version, Valid: true}})
	if err != nil {
		return modelbiz.Version{}, err
	}
	return modelbiz.Version{TenantID: tenantID, ID: versionID, ModelID: uuid.UUID(row.ModelID.Bytes).String(), ExternalModelID: row.ExternalModelID, Version: row.Version, Format: row.Format, Status: row.Status, ArtifactProvider: row.ArtifactProvider, ArtifactReference: row.ArtifactReference, ArtifactSHA256: row.ArtifactSha256, EngineType: row.EngineType, StartupCommand: row.StartupCommand}, nil
}
