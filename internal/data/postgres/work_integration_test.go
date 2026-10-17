package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
	workbiz "github.com/liangzai006/ani-model-service/internal/biz/work"
)

func TestWorkStoreLeaseRecoveryPostgres(t *testing.T) {
	dsn := os.Getenv("MODEL_DATABASE_URL")
	if dsn == "" {
		t.Skip("MODEL_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	tenant, modelID, taskID := uuid.New(), uuid.New(), uuid.New()
	_, err = pool.Exec(ctx, "INSERT INTO public.models(tenant_id,id,name,source,status) VALUES($1,$2,$3,'upload','pending')", tenant, modelID, "it-"+taskID.String())
	if err != nil {
		t.Fatal(err)
	}
	store := NewWorkStore(pool)
	_, err = store.Create(ctx, workTask(tenant, modelID, taskID))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(ctx, tenant.String(), taskID.String(), "worker-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Renew(ctx, tenant.String(), taskID.String(), "worker-a", claimed.LeaseEpoch, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, tenant.String(), taskID.String(), "worker-old", claimed.LeaseEpoch); err == nil {
		t.Fatal("stale owner unexpectedly completed task")
	}
	if err := store.Complete(ctx, tenant.String(), taskID.String(), "worker-a", claimed.LeaseEpoch); err != nil {
		t.Fatal(err)
	}
}

// An interrupted process leaves an importing row leased; after expiry the
// next process must be able to claim it with a newer fencing epoch.
func TestWorkStoreExpiredLeaseCanBeReclaimedPostgres(t *testing.T) {
	dsn := os.Getenv("MODEL_DATABASE_URL")
	if dsn == "" {
		t.Skip("MODEL_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	tenant, modelID, taskID := uuid.New(), uuid.New(), uuid.New()
	_, err = pool.Exec(ctx, "INSERT INTO public.models(tenant_id,id,name,source,status) VALUES($1,$2,$3,'upload','pending')", tenant, modelID, "reclaim-"+taskID.String())
	if err != nil {
		t.Fatal(err)
	}
	store := NewWorkStore(pool)
	created, err := store.Create(ctx, workTask(tenant, modelID, taskID))
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Claim(ctx, tenant.String(), created.ID, "crashed-worker", 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	second, err := store.Claim(ctx, tenant.String(), created.ID, "restarted-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if second.LeaseEpoch <= first.LeaseEpoch || second.LeaseOwner != "restarted-worker" {
		t.Fatalf("reclaim=%+v first=%+v", second, first)
	}
	if err := store.Complete(ctx, tenant.String(), created.ID, "crashed-worker", first.LeaseEpoch); err == nil {
		t.Fatal("stale crashed worker completed task")
	}
	if err := store.Complete(ctx, tenant.String(), created.ID, "restarted-worker", second.LeaseEpoch); err != nil {
		t.Fatal(err)
	}
}

func TestWorkStoreRemoteImportWithoutModelBindingPostgres(t *testing.T) {
	dsn := os.Getenv("MODEL_DATABASE_URL")
	if dsn == "" {
		t.Skip("MODEL_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tenant, taskID := uuid.New(), uuid.New()
	store := NewWorkStore(pool)
	task := workbiz.Task{TenantID: tenant.String(), ID: taskID.String(), TaskType: "huggingface", Source: "huggingface", RepoID: "org/model", Revision: "main", IdempotencyKey: "remote-" + taskID.String()}
	created, err := store.Create(ctx, task)
	if err != nil {
		t.Fatal(err)
	}
	if created.ModelID != "" || created.VersionID != "" || created.Status != workbiz.Pending {
		t.Fatalf("unexpected unbound import task: %+v", created)
	}
	replay, err := store.Create(ctx, task)
	if err != nil || replay.ID != created.ID {
		t.Fatalf("same idempotency request did not replay original task: task=%+v err=%v", replay, err)
	}
	task.RepoID = "org/other-model"
	if _, err := store.Create(ctx, task); err == nil {
		t.Fatal("different payload unexpectedly replayed")
	} else if err == pgx.ErrNoRows {
		t.Fatalf("conflict returned not-found: %v", err)
	}
}

func TestWorkStoreGetAndRetryFailedPostgres(t *testing.T) {
	dsn := os.Getenv("MODEL_DATABASE_URL")
	if dsn == "" {
		t.Skip("MODEL_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tenant, modelID, taskID := uuid.New(), uuid.New(), uuid.New()
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM public.model_import_tasks WHERE tenant_id=$1", tenant)
		_, _ = pool.Exec(context.Background(), "DELETE FROM public.models WHERE tenant_id=$1", tenant)
	}()
	if _, err = pool.Exec(ctx, "INSERT INTO public.models(tenant_id,id,name,source,status) VALUES($1,$2,$3,'upload','pending')", tenant, modelID, "retry-"+taskID.String()); err != nil {
		t.Fatal(err)
	}
	versionID := uuid.New()
	versions := NewVersionStore(pool)
	if _, err = versions.CreateVersion(ctx, modelbiz.Version{TenantID: tenant.String(), ID: versionID.String(), ModelID: modelID.String(), Version: "v1", Format: "gguf"}); err != nil {
		t.Fatal(err)
	}
	store := NewWorkStore(pool)
	created, err := store.Create(ctx, workbiz.Task{TenantID: tenant.String(), ModelID: modelID.String(), VersionID: versionID.String(), ID: taskID.String(), TaskType: "upload", Source: "upload", IdempotencyKey: "retry-" + taskID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE public.model_versions SET status='error', error_message='checksum mismatch' WHERE tenant_id=$1 AND id=$2", tenant, versionID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE public.model_import_tasks SET status='failed', attempt_count=5, progress_pct=73, error_message='checksum mismatch' WHERE tenant_id=$1 AND id=$2", tenant, taskID); err != nil {
		t.Fatal(err)
	}
	failed, err := store.Get(ctx, tenant.String(), created.ID)
	if err != nil || failed.Status != workbiz.Failed || failed.ProgressPct != 73 || failed.ErrorMessage != "checksum mismatch" {
		t.Fatalf("get failed task=%+v err=%v", failed, err)
	}
	retried, err := store.RetryFailed(ctx, tenant.String(), created.ID)
	if err != nil || retried.Status != workbiz.Pending || retried.AttemptCount != 0 || retried.ProgressPct != 0 || retried.ErrorMessage != "" {
		t.Fatalf("retry task=%+v err=%v", retried, err)
	}
	var versionStatus, versionError string
	if err := pool.QueryRow(ctx, "SELECT status, error_message FROM public.model_versions WHERE tenant_id=$1 AND id=$2", tenant, versionID).Scan(&versionStatus, &versionError); err != nil {
		t.Fatal(err)
	}
	if versionStatus != "pending" || versionError != "" {
		t.Fatalf("retry version status=%q error=%q, want pending and empty", versionStatus, versionError)
	}
}

func TestVersionStoreMarkErrorAggregatePostgres(t *testing.T) {
	dsn := os.Getenv("MODEL_DATABASE_URL")
	if dsn == "" {
		t.Skip("MODEL_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	tenant := uuid.New()
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM public.model_import_tasks WHERE tenant_id=$1", tenant)
		_, _ = pool.Exec(context.Background(), "DELETE FROM public.models WHERE tenant_id=$1", tenant)
	}()
	versions := NewVersionStore(pool)
	var versionStatus string
	createModel := func(name, status string) uuid.UUID {
		modelID := uuid.New()
		if _, err := pool.Exec(ctx, "INSERT INTO public.models(tenant_id,id,name,source,status) VALUES($1,$2,$3,'upload',$4)", tenant, modelID, name, status); err != nil {
			t.Fatal(err)
		}
		return modelID
	}
	createVersion := func(modelID uuid.UUID, version, status string) uuid.UUID {
		versionID := uuid.New()
		if _, err := versions.CreateVersion(ctx, modelbiz.Version{TenantID: tenant.String(), ID: versionID.String(), ModelID: modelID.String(), Version: version, Format: "gguf"}); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE public.model_versions SET status=$3 WHERE tenant_id=$1 AND id=$2", tenant, versionID, status); err != nil {
			t.Fatal(err)
		}
		return versionID
	}

	withoutReady := createModel("error-"+uuid.NewString(), "pending")
	withoutReadyVersion := createVersion(withoutReady, "v1", "pending")
	if err := versions.MarkErrorAndMaybeModel(ctx, tenant.String(), withoutReadyVersion.String(), "provider failed"); err != nil {
		t.Fatal(err)
	}
	var modelStatus string
	if err := pool.QueryRow(ctx, "SELECT status FROM public.models WHERE tenant_id=$1 AND id=$2", tenant, withoutReady).Scan(&modelStatus); err != nil {
		t.Fatal(err)
	}
	if modelStatus != "error" {
		t.Fatalf("model without ready version = %q, want error", modelStatus)
	}

	withReady := createModel("ready-"+uuid.NewString(), "ready")
	failingVersion := createVersion(withReady, "v1", "pending")
	_ = createVersion(withReady, "v2", "ready")
	if err := versions.MarkErrorAndMaybeModel(ctx, tenant.String(), failingVersion.String(), "provider failed"); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT status FROM public.models WHERE tenant_id=$1 AND id=$2", tenant, withReady).Scan(&modelStatus); err != nil {
		t.Fatal(err)
	}
	if modelStatus != "ready" {
		t.Fatalf("model with ready version = %q, want ready", modelStatus)
	}
	readyTarget := createVersion(withReady, "v3", "ready")
	if err := versions.MarkErrorAndMaybeModel(ctx, tenant.String(), readyTarget.String(), "must not overwrite"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("ready version transition err=%v, want pgx.ErrNoRows", err)
	}

	deletedModel := createModel("deleted-"+uuid.NewString(), "deleted")
	deletedVersion := createVersion(deletedModel, "v1", "pending")
	if err := versions.MarkErrorAndMaybeModel(ctx, tenant.String(), deletedVersion.String(), "must not overwrite model"); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT status FROM public.models WHERE tenant_id=$1 AND id=$2", tenant, deletedModel).Scan(&modelStatus); err != nil {
		t.Fatal(err)
	}
	if modelStatus != "deleted" {
		t.Fatalf("deleted model status=%q, want deleted", modelStatus)
	}

	deletedVersion = createVersion(deletedModel, "v2", "deleted")
	if err := versions.MarkErrorAndMaybeModel(ctx, tenant.String(), deletedVersion.String(), "must not overwrite"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("deleted version transition err=%v, want pgx.ErrNoRows", err)
	}
	if err := pool.QueryRow(ctx, "SELECT status FROM public.model_versions WHERE tenant_id=$1 AND id=$2", tenant, deletedVersion).Scan(&versionStatus); err != nil {
		t.Fatal(err)
	}
	if versionStatus != "deleted" {
		t.Fatalf("deleted version status=%q, want deleted", versionStatus)
	}

	if err := versions.MarkErrorAndMaybeModel(ctx, uuid.New().String(), withoutReadyVersion.String(), "cross tenant"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant transition err=%v, want pgx.ErrNoRows", err)
	}
}

func workTask(t, m, id uuid.UUID) workbiz.Task {
	return workbiz.Task{TenantID: t.String(), ModelID: m.String(), ID: id.String(), TaskType: "upload", Source: "upload", IdempotencyKey: "it-" + id.String()}
}
