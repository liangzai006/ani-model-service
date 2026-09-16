package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	workbiz "github.com/zhangzhe-ctrl/ani-model-service/internal/biz/work"
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
func workTask(t, m, id uuid.UUID) workbiz.Task {
	return workbiz.Task{TenantID: t.String(), ModelID: m.String(), ID: id.String(), TaskType: "upload", Source: "upload", IdempotencyKey: "it-" + id.String()}
}
