package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	modelbiz "github.com/zhangzhe-ctrl/ani-model-service/internal/biz/model"
)

func TestModelAndVersionIdempotencyPostgres(t *testing.T) {
	dsn := os.Getenv("MODEL_DATABASE_URL")
	if dsn == "" {
		t.Skip("MODEL_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tenant, modelID, replayModelID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	models := NewModelStore(pool)
	first, err := models.CreateModel(ctx, tenant, modelID, "idem-model", "", "", "upload", []byte(`[]`), "model-key-"+modelID)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := models.CreateModel(ctx, tenant, replayModelID, "idem-model", "", "", "upload", []byte(`[]`), "model-key-"+modelID)
	if err != nil || uuid.UUID(replay.ID.Bytes).String() != uuid.UUID(first.ID.Bytes).String() {
		t.Fatalf("model replay mismatch: first=%+v replay=%+v err=%v", first, replay, err)
	}
	if _, err := models.CreateModel(ctx, tenant, uuid.NewString(), "different", "", "", "upload", []byte(`[]`), "model-key-"+modelID); err == nil {
		t.Fatal("different model payload unexpectedly replayed")
	}

	versions := NewVersionStore(pool)
	versionID := uuid.NewString()
	v := modelbiz.Version{TenantID: tenant, ID: versionID, ModelID: modelID, Version: "v1", Format: "safetensors", ArtifactSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", IdempotencyKey: "version-key-" + versionID}
	created, err := versions.CreateVersion(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	v.ID = uuid.NewString()
	replayed, err := versions.CreateVersion(ctx, v)
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("version replay mismatch: created=%+v replayed=%+v err=%v", created, replayed, err)
	}
	v.Version = "v2"
	if _, err := versions.CreateVersion(ctx, v); err == nil {
		t.Fatal("different version payload unexpectedly replayed")
	}
}
