package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	workbiz "github.com/liangzai006/ani-model-service/internal/biz/work"
	"github.com/liangzai006/ani-model-service/internal/data/importer"
)

// ImportBinder resolves provider metadata to existing Model-owned records. It
// never creates a model or version from an untrusted repository name.
type ImportBinder struct {
	q         *Queries
	providers importer.Registry
}

func NewImportBinder(db DBTX, providers importer.Registry) *ImportBinder {
	return &ImportBinder{q: New(db), providers: providers}
}

func (b *ImportBinder) ResolveBinding(ctx context.Context, task workbiz.Task) (string, string, error) {
	provider, err := b.providers.Resolve(task.Source)
	if err != nil {
		return "", "", err
	}
	metadataSource, ok := provider.(importer.MetadataSource)
	if !ok {
		return "", "", fmt.Errorf("provider %q does not expose metadata", task.Source)
	}
	metadata, err := metadataSource.ResolveMetadata(ctx, importer.Request{RepoID: task.RepoID, Revision: task.Revision})
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(metadata.ExternalModelID) == "" || strings.TrimSpace(metadata.Version) == "" {
		return "", "", fmt.Errorf("provider metadata is incomplete")
	}
	tenant, err := uuid.Parse(task.TenantID)
	if err != nil {
		return "", "", fmt.Errorf("tenant_id: %w", err)
	}
	modelRow, err := b.q.GetModelByExternalID(ctx, GetModelByExternalIDParams{TenantID: uuidType(tenant), ModelID: metadata.ExternalModelID})
	if err != nil {
		return "", "", err
	}
	modelID := uuid.UUID(modelRow.ID.Bytes).String()
	versionRow, err := b.q.GetModelVersionForImport(ctx, GetModelVersionForImportParams{TenantID: uuidType(tenant), ModelID: modelRow.ID, Version: metadata.Version})
	if err != nil {
		return "", "", err
	}
	return modelID, uuid.UUID(versionRow.Bytes).String(), nil
}
