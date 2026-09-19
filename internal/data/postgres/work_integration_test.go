package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	store := NewWorkStore(pool)
	created, err := store.Create(ctx, workbiz.Task{TenantID: tenant.String(), ModelID: modelID.String(), ID: taskID.String(), TaskType: "upload", Source: "upload", IdempotencyKey: "retry-" + taskID.String()})
	if err != nil {
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
}

func workTask(t, m, id uuid.UUID) workbiz.Task {
	return workbiz.Task{TenantID: t.String(), ModelID: m.String(), ID: id.String(), TaskType: "upload", Source: "upload", IdempotencyKey: "it-" + id.String()}
}
