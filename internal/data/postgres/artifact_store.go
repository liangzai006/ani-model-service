package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
)

// ArtifactStore persists object metadata only. Storage bytes and lifecycle
// remain owned by the external Storage service.
type ArtifactStore struct{ pool DBTX }

func NewArtifactStore(db DBTX) *ArtifactStore { return &ArtifactStore{pool: db} }

func (s *ArtifactStore) CreateArtifact(ctx context.Context, a modelbiz.Artifact) (modelbiz.Artifact, error) {
	if err := a.Validate(); err != nil {
		return modelbiz.Artifact{}, err
	}
	tenant, id, err := parseModelIDs(a.TenantID, a.ID)
	if err != nil {
		return modelbiz.Artifact{}, err
	}
	versionID, err := uuid.Parse(a.ModelVersionID)
	if err != nil {
		return modelbiz.Artifact{}, fmt.Errorf("model_version_id: %w", err)
	}
	r, err := New(s.pool).CreateModelArtifact(ctx, CreateModelArtifactParams{
		TenantID: uuidType(tenant), ID: uuidType(id), ModelVersionID: uuidType(versionID),
		Provider: a.Provider, Reference: a.Reference, Format: a.Format, SizeBytes: a.SizeBytes,
		Sha256: a.SHA256, IsEncrypted: a.IsEncrypted, EncryptAlgo: a.EncryptAlgo,
	})
	if err != nil {
		return modelbiz.Artifact{}, err
	}
	return artifactFromRow(r), nil
}

func (s *ArtifactStore) GetArtifact(ctx context.Context, tenantID, versionID string) (modelbiz.Artifact, error) {
	tenant, err := uuid.Parse(tenantID)
	if err != nil {
		return modelbiz.Artifact{}, fmt.Errorf("tenant_id: %w", err)
	}
	version, err := uuid.Parse(versionID)
	if err != nil {
		return modelbiz.Artifact{}, fmt.Errorf("model_version_id: %w", err)
	}
	r, err := New(s.pool).GetModelArtifact(ctx, GetModelArtifactParams{TenantID: uuidType(tenant), ModelVersionID: uuidType(version)})
	if err != nil {
		return modelbiz.Artifact{}, err
	}
	return artifactFromRow(r), nil
}

func artifactFromRow(r ModelArtifact) modelbiz.Artifact {
	return modelbiz.Artifact{
		TenantID: uuid.UUID(r.TenantID.Bytes).String(), ID: uuid.UUID(r.ID.Bytes).String(),
		ModelVersionID: uuid.UUID(r.ModelVersionID.Bytes).String(), Provider: r.Provider,
		Reference: r.Reference, Format: r.Format, SizeBytes: r.SizeBytes, SHA256: r.Sha256,
		IsEncrypted: r.IsEncrypted, EncryptAlgo: r.EncryptAlgo,
	}
}
