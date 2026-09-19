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
	"github.com/liangzai006/ani-model-service/internal/biz/idempotency"
	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
)

type ModelStore struct{ pool DBTX }

func NewModelStore(pool DBTX) *ModelStore { return &ModelStore{pool: pool} }

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

func (s *ModelStore) ListModels(ctx context.Context, tenantID string, options modelbiz.ListOptions) ([]ListModelsRow, error) {
	tenant, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant_id: %w", err)
	}
	beforeTime, beforeID, err := listBoundary(options)
	if err != nil {
		return nil, err
	}
	return New(s.pool).ListModels(ctx, ListModelsParams{TenantID: uuidType(tenant), StatusFilter: options.Status, SourceFilter: options.Source, Capability: options.Capability, Keyword: options.Keyword, PageLimit: options.Limit, BeforeCreatedAt: beforeTime, BeforeID: beforeID})
}

func listBoundary(options modelbiz.ListOptions) (pgtype.Timestamptz, pgtype.UUID, error) {
	if options.Limit < 1 || options.Limit > 1001 {
		return pgtype.Timestamptz{}, pgtype.UUID{}, fmt.Errorf("invalid list limit")
	}
	if options.BeforeID == "" && options.BeforeCreatedAt.IsZero() {
		return pgtype.Timestamptz{}, pgtype.UUID{}, nil
	}
	id, err := uuid.Parse(options.BeforeID)
	if err != nil || id == uuid.Nil || options.BeforeCreatedAt.IsZero() {
		return pgtype.Timestamptz{}, pgtype.UUID{}, fmt.Errorf("invalid list boundary")
	}
	return pgtype.Timestamptz{Time: options.BeforeCreatedAt, Valid: true}, uuidType(id), nil
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
	return NewVersionStore(s.pool).GetVersion(ctx, tenantID, versionID)
}
