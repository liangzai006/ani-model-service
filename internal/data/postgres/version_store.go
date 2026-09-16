package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zhangzhe-ctrl/ani-model-service/internal/biz/idempotency"
	modelbiz "github.com/zhangzhe-ctrl/ani-model-service/internal/biz/model"
)

type VersionStore struct{ pool DBTX }

func NewVersionStore(db DBTX) *VersionStore { return &VersionStore{pool: db} }
func (s *VersionStore) GetVersion(ctx context.Context, tenant, id string) (modelbiz.Version, error) {
	t, vid, err := parseModelIDs(tenant, id)
	if err != nil {
		return modelbiz.Version{}, err
	}
	r, err := New(s.pool).GetReadyModelVersion(ctx, GetReadyModelVersionParams{TenantID: uuidType(t), ID: uuidType(vid)})
	if err != nil {
		return modelbiz.Version{}, err
	}
	var args []string
	_ = json.Unmarshal(r.StartupArgs, &args)
	return modelbiz.Version{TenantID: tenant, ID: id, ModelID: uuid.UUID(r.ModelID.Bytes).String(), ExternalModelID: r.ExternalModelID, Version: r.Version, Format: r.Format, Status: r.Status, ArtifactProvider: r.ArtifactProvider, ArtifactReference: r.ArtifactReference, ArtifactSHA256: r.ArtifactSha256, EngineType: r.EngineType, StartupCommand: r.StartupCommand, StartupArgs: args}, nil
}

func (s *VersionStore) GetVersionByExternalRef(ctx context.Context, tenant, externalModelID, version string) (modelbiz.Version, error) {
	t, err := uuid.Parse(tenant)
	if err != nil {
		return modelbiz.Version{}, fmt.Errorf("tenant_id: %w", err)
	}
	r, err := New(s.pool).GetReadyModelVersionByExternalRef(ctx, GetReadyModelVersionByExternalRefParams{TenantID: uuidType(t), ModelID: externalModelID, Version: version})
	if err != nil {
		return modelbiz.Version{}, err
	}
	var args []string
	_ = json.Unmarshal(r.StartupArgs, &args)
	return modelbiz.Version{TenantID: tenant, ID: uuid.UUID(r.ID.Bytes).String(), ModelID: uuid.UUID(r.ModelID.Bytes).String(), ExternalModelID: r.ExternalModelID, Version: r.Version, Format: r.Format, Status: r.Status, ArtifactProvider: r.ArtifactProvider, ArtifactReference: r.ArtifactReference, ArtifactSHA256: r.ArtifactSha256, EngineType: r.EngineType, StartupCommand: r.StartupCommand, StartupArgs: args}, nil
}
func (s *VersionStore) CreateVersion(ctx context.Context, v modelbiz.Version) (modelbiz.Version, error) {
	t, id, err := parseModelIDs(v.TenantID, v.ID)
	if err != nil {
		return modelbiz.Version{}, err
	}
	mid, err := uuid.Parse(v.ModelID)
	if err != nil {
		return modelbiz.Version{}, fmt.Errorf("model_id: %w", err)
	}
	args, _ := json.Marshal(v.StartupArgs)
	fingerprintBytes, _ := json.Marshal(struct {
		ModelID, Version, Format, ArtifactReference, ArtifactSHA256, EngineType, StartupCommand string
		StartupArgs                                                                             []string
	}{v.ModelID, v.Version, v.Format, v.ArtifactReference, v.ArtifactSHA256, v.EngineType, v.StartupCommand, v.StartupArgs})
	fingerprint := idempotency.Fingerprint(fingerprintBytes)
	q := New(s.pool)
	if v.IdempotencyKey != "" {
		if existing, lookupErr := q.GetModelVersionByIdempotency(ctx, GetModelVersionByIdempotencyParams{TenantID: uuidType(t), IdempotencyKey: v.IdempotencyKey}); lookupErr == nil {
			if existing.RequestFingerprint != fingerprint {
				return modelbiz.Version{}, &pgconn.PgError{Code: "23505", Message: "idempotency key payload conflict"}
			}
			return versionFromRow(existing, v.TenantID), nil
		} else if !errors.Is(lookupErr, pgx.ErrNoRows) {
			return modelbiz.Version{}, lookupErr
		}
	}
	r, err := q.CreateModelVersion(ctx, CreateModelVersionParams{TenantID: uuidType(t), ID: uuidType(id), ModelID: uuidType(mid), Version: v.Version, Format: v.Format, ChecksumSha256: v.ArtifactSHA256, EngineType: v.EngineType, StartupCommand: v.StartupCommand, StartupArgs: args, IdempotencyKey: v.IdempotencyKey, RequestFingerprint: fingerprint})
	if err != nil {
		return modelbiz.Version{}, err
	}
	return versionFromRow(r, v.TenantID), nil
}
func (s *VersionStore) ListVersions(ctx context.Context, tenant, model string, limit int32) ([]modelbiz.Version, error) {
	t, mid, err := parseModelIDs(tenant, model)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rs, err := New(s.pool).ListModelVersions(ctx, ListModelVersionsParams{TenantID: uuidType(t), ModelID: uuidType(mid), Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]modelbiz.Version, len(rs))
	for i, r := range rs {
		var args []string
		_ = json.Unmarshal(r.StartupArgs, &args)
		out[i] = modelbiz.Version{TenantID: tenant, ID: uuid.UUID(r.ID.Bytes).String(), ModelID: uuid.UUID(r.ModelID.Bytes).String(), ExternalModelID: r.ExternalModelID, Version: r.Version, Format: r.Format, Status: r.Status, ArtifactSHA256: r.ChecksumSha256, EngineType: r.EngineType, StartupCommand: r.StartupCommand, StartupArgs: args}
	}
	return out, nil
}

func (s *VersionStore) MarkReady(ctx context.Context, tenant, version string) error {
	t, id, err := parseModelIDs(tenant, version)
	if err != nil {
		return err
	}
	n, err := New(s.pool).MarkModelVersionReady(ctx, MarkModelVersionReadyParams{TenantID: uuidType(t), ID: uuidType(id)})
	if err != nil {
		return err
	}
	if n != 1 {
		// A replay may reach finalization after a previous worker already
		// marked this version ready. Treat that state as idempotent success;
		// all other zero-row outcomes remain not-found/conflict to the caller.
		if _, getErr := s.GetVersion(ctx, tenant, version); getErr == nil {
			return nil
		}
		return pgx.ErrNoRows
	}
	return nil
}

func (s *VersionStore) MarkError(ctx context.Context, tenant, version, message string) error {
	t, id, err := parseModelIDs(tenant, version)
	if err != nil {
		return err
	}
	n, err := New(s.pool).MarkModelVersionError(ctx, MarkModelVersionErrorParams{TenantID: uuidType(t), ID: uuidType(id), ErrorMessage: message})
	if err != nil {
		return err
	}
	if n != 1 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *VersionStore) SetChecksum(ctx context.Context, tenant, version, checksum string, size int64) error {
	t, id, err := parseModelIDs(tenant, version)
	if err != nil {
		return err
	}
	n, err := New(s.pool).SetModelVersionChecksum(ctx, SetModelVersionChecksumParams{TenantID: uuidType(t), ID: uuidType(id), ChecksumSha256: checksum, Column4: size})
	if err != nil {
		return err
	}
	if n != 1 {
		return pgx.ErrNoRows
	}
	return nil
}
func versionFromRow(r ModelVersion, tenant string) modelbiz.Version {
	var args []string
	_ = json.Unmarshal(r.StartupArgs, &args)
	return modelbiz.Version{TenantID: tenant, ID: uuid.UUID(r.ID.Bytes).String(), ModelID: uuid.UUID(r.ModelID.Bytes).String(), Version: r.Version, Format: r.Format, Status: r.Status, ArtifactSHA256: r.ChecksumSha256, EngineType: r.EngineType, StartupCommand: r.StartupCommand, StartupArgs: args}
}
