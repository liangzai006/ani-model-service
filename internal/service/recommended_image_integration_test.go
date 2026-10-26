package service

import (
	"context"
	"os"
	"testing"

	kratoserrors "github.com/go-kratos/kratos/v3/errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
	"github.com/liangzai006/ani-model-service/internal/biz/storage"
	"github.com/liangzai006/ani-model-service/internal/data/postgres"
	"github.com/liangzai006/ani-model-service/internal/identity"
)

// PostgreSQL and Model business paths are real. Only the external object
// storage boundary is replaced so this remains independent of S3 publication.
func TestRecommendedImageRegistrationInheritancePostgres(t *testing.T) {
	dsn := os.Getenv("MODEL_DATABASE_URL")
	if dsn == "" {
		t.Skip("MODEL_DATABASE_URL not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tenant := uuid.NewString()
	t.Cleanup(func() {
		for _, table := range []string{"trained_artifact_registrations", "model_artifacts", "model_versions", "models"} {
			if _, err := pool.Exec(ctx, "DELETE FROM public."+table+" WHERE tenant_id=$1", tenant); err != nil {
				t.Error(err)
			}
		}
	})
	ctx = identity.WithPrincipal(ctx, identity.Principal{TenantID: tenant, Actor: "modeldev", Workload: "modeldev-service"})
	versions := postgres.NewVersionStore(pool)
	catalog := postgres.NewCatalogStore(postgres.NewModelStore(pool))
	storageBoundary := &trainedArtifactStorageFake{exists: true, verification: storage.ObjectVerification{SizeBytes: 4, Matches: true}}
	s := NewModelServiceWithDependencies(versions, versions, catalog, storageBoundary, nil)
	s.SetArtifactStore(postgres.NewArtifactStore(pool))
	s.SetTrainedArtifactRegistrar(postgres.NewTrainedArtifactStore(pool))
	owner, err := s.CreateModel(ctx, &modelv1.CreateModelRequest{TenantId: tenant, Name: "source-" + tenant[:8], IdempotencyKey: "source-model"})
	if err != nil {
		t.Fatal(err)
	}
	request := &modelv1.CreateModelVersionRequest{TenantId: tenant, ModelId: owner.GetId(), Version: "source-v1", Format: "safetensors", StoragePath: tenant + "/source/model.tar", ChecksumSha256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SizeBytes: 4, IdempotencyKey: "source-version", EngineType: "vllm", EngineImage: "registry/vllm@sha256:frozen", StartupCommand: "serve /models", RecommendedImageId: "image-a", RecommendedImageScope: "platform"}
	source, err := s.CreateModelVersion(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if source.GetStatus() != "ready" || source.GetRecommendedImageId() != "image-a" {
		t.Fatalf("source=%+v", source)
	}
	replayed, err := s.CreateModelVersion(ctx, request)
	if err != nil || replayed.GetId() != source.GetId() {
		t.Fatalf("version replay=%+v err=%v", replayed, err)
	}
	request.RecommendedImageId = "image-b"
	if _, err := s.CreateModelVersion(ctx, request); kratoserrors.Code(err) != 409 {
		t.Fatalf("changed image=%v", err)
	}
	listed, err := s.ListModels(ctx, &modelv1.ListModelsRequest{TenantId: tenant})
	if err != nil || len(listed.GetItems()) != 1 || len(listed.GetItems()[0].GetVersions()) != 1 || listed.GetItems()[0].GetVersions()[0].GetRecommendedImageId() != "image-a" {
		t.Fatalf("catalog=%+v err=%v", listed, err)
	}
	in := trainedArtifactRequest()
	in.TenantId, in.TargetModelId, in.SourceModelVersionId = tenant, owner.GetId(), source.GetId()
	in.EngineType, in.EngineImage, in.StartupCommand = "", "", ""
	prepared, err := s.PrepareTrainedArtifact(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE public.model_versions SET recommended_image_id='image-b', recommended_image_scope='tenant', engine_image='registry/vllm@sha256:changed' WHERE tenant_id=$1 AND id=$2`, tenant, source.GetId()); err != nil {
		t.Fatal(err)
	}
	// Recreate the service and registrar to prove the frozen selection is durable.
	s = NewModelServiceWithDependencies(versions, versions, catalog, storageBoundary, nil)
	s.SetTrainedArtifactRegistrar(postgres.NewTrainedArtifactStore(pool))
	again, err := s.PrepareTrainedArtifact(ctx, in)
	if err != nil || again.GetRegistrationId() != prepared.GetRegistrationId() {
		t.Fatalf("prepare replay=%+v err=%v", again, err)
	}
	completed, err := s.CompleteTrainedArtifact(ctx, &modelv1.CompleteTrainedArtifactRequest{TenantId: tenant, RegistrationId: prepared.GetRegistrationId(), IdempotencyKey: in.GetIdempotencyKey()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetModelVersion(ctx, &modelv1.GetModelVersionRequest{TenantId: tenant, ModelVersionId: completed.GetModelVersionId()})
	if err != nil || got.GetVersion().GetRecommendedImageId() != "image-a" || got.GetVersion().GetRecommendedImageScope() != "platform" || got.GetVersion().GetEngineImage() != "registry/vllm@sha256:frozen" {
		t.Fatalf("completed=%+v err=%v", got, err)
	}
	in.IdempotencyKey, in.Version = "aborted-registration", "aborted-version"
	aborted, err := s.PrepareTrainedArtifact(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AbortTrainedArtifact(ctx, &modelv1.AbortTrainedArtifactRequest{TenantId: tenant, RegistrationId: aborted.GetRegistrationId(), Reason: "cancel test registration"}); err != nil {
		t.Fatal(err)
	}
	var state string
	var aggregateCount int
	if err := pool.QueryRow(ctx, `SELECT status FROM public.trained_artifact_registrations WHERE tenant_id=$1 AND id=$2`, tenant, aborted.GetRegistrationId()).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.model_versions WHERE tenant_id=$1`, tenant).Scan(&aggregateCount); err != nil {
		t.Fatal(err)
	}
	if state != "aborted" || aggregateCount != 2 {
		t.Fatalf("abort state=%s aggregate count=%d", state, aggregateCount)
	}
}
