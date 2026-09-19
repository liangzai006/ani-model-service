package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
)

func TestArtifactStoreTenantIsolationPostgres(t *testing.T) {
	dsn := os.Getenv("MODEL_DATABASE_URL")
	if dsn == "" {
		t.Skip("MODEL_DATABASE_URL not configured")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx := context.Background()
	tenant := uuid.NewString()
	otherTenant := uuid.NewString()
	modelID, versionID, artifactID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	ms := NewModelStore(pool)
	if _, err := ms.CreateModel(ctx, tenant, modelID, "artifact-test", "", "", "upload", []byte(`[]`), ""); err != nil {
		t.Fatal(err)
	}
	vs := NewVersionStore(pool)
	if _, err := vs.CreateVersion(ctx, modelbiz.Version{TenantID: tenant, ID: versionID, ModelID: modelID, Version: "v1", Format: "safetensors", ArtifactSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}); err != nil {
		t.Fatal(err)
	}
	as := NewArtifactStore(pool)
	want := modelbiz.Artifact{TenantID: tenant, ID: artifactID, ModelVersionID: versionID, Provider: "storage", Reference: "obj://artifact-test", Format: "safetensors", SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	got, err := as.CreateArtifact(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != artifactID || got.TenantID != tenant {
		t.Fatalf("unexpected artifact: %+v", got)
	}
	if _, err := as.GetArtifact(ctx, otherTenant, versionID); err == nil || err != pgx.ErrNoRows {
		t.Fatalf("cross-tenant read error = %v, want pgx.ErrNoRows", err)
	}
	if err := vs.MarkReady(ctx, tenant, versionID); err != nil {
		t.Fatalf("matching artifact did not allow ready transition: %v", err)
	}
	ready, err := vs.GetVersion(ctx, tenant, versionID)
	if err != nil || ready.Status != "ready" || ready.ArtifactReference != want.Reference || ready.ArtifactSHA256 != want.SHA256 {
		t.Fatalf("ready version mismatch: %+v err=%v", ready, err)
	}
	secondVersion := uuid.NewString()
	if _, err := vs.CreateVersion(ctx, modelbiz.Version{TenantID: tenant, ID: secondVersion, ModelID: modelID, Version: "v2", Format: "safetensors", ArtifactSHA256: want.SHA256}); err != nil {
		t.Fatal(err)
	}
	if err := vs.MarkReady(ctx, tenant, secondVersion); err == nil {
		t.Fatal("version without artifact unexpectedly became ready")
	}
}
