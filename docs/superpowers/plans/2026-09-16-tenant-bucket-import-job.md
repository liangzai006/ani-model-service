# Tenant Bucket Import Job Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Start durable model import jobs immediately after `ImportModel`, ensure a tenant-named MinIO bucket exists before download, and preserve restart recovery with detailed logs.

**Architecture:** `ImportModel` commits the PostgreSQL task first, then signals an in-process worker wake channel. The worker keeps its PostgreSQL due scan as the durable fallback, claims the task with the existing lease/CAS fencing, calls a tenant-bucket ensure operation, and streams the provider response into that bucket. The provisioning command uses the same tenant bucket naming for explicit pre-provisioning but is never launched by the service.

**Tech Stack:** Go, PostgreSQL/sqlc, MinIO SDK, Kratos gRPC, `log/slog`, existing import worker and Storage ports.

## Global Constraints

- The Model service remains an independent repository and uses the pinned Kratos layout commit `fd18422211c741dd5242d2992c3d307d522aa28c`.
- PostgreSQL uses the `public` schema, no RLS, and every business query explicitly limits `tenant_id`.
- PostgreSQL remains the durable source of import tasks; wake signals are an optimization and cannot replace due scans.
- Bucket names are validated tenant UUIDs; runtime creation is idempotent and does not delete buckets, PVCs, or manage MinIO service lifecycle.
- Provider, Storage, IAM, Inference, and Kubernetes remain external boundaries; no old ANI tables or runtime are copied.
- Secrets, signed URLs, and model bytes must not be written to logs.
- No frontend, remote repository, commit, push, or destructive cleanup is performed.

### Task 1: Add tenant bucket ensure capability

**Files:**
- Modify: `internal/biz/storage/storage.go`
- Modify: `internal/data/storage/minio.go`
- Modify: `internal/data/storage/minio_test.go`
- Modify: `cmd/minio-provision/main.go`
- Modify: `docs/deploy/minio-env.yaml`

**Interfaces:**
- Produces `storage.BucketManager` with `EnsureBucket(context.Context, string) error`.
- `MinIOAdapter` resolves the bucket from the tenant UUID and performs `BucketExists`, followed by `MakeBucket` only when absent; concurrent `BucketAlreadyExists` is success.
- The provisioning command accepts `ANI_MINIO_TENANT_ID` and ensures that tenant bucket; `ANI_MINIO_BUCKET` remains available only for legacy shared-bucket compatibility during migration.

- [x] Write tests for valid tenant bucket selection, existing bucket no-op, missing bucket creation, and invalid tenant rejection.
- [x] Run the focused tests and verify the new tests failed before implementation.
- [x] Implement the manager and MinIO adapter with no bucket deletion or lifecycle operations.
- [x] Run the focused tests and `gofmt -w` on changed Go files.

### Task 2: Wake the durable import worker after task commit

**Files:**
- Modify: `internal/worker/import_worker.go`
- Modify: `internal/service/model.go`
- Modify: `cmd/ani-model-service/main.go`
- Modify: `internal/worker/import_worker_test.go`
- Modify: `internal/service/model_test.go`

**Interfaces:**
- Produces `worker.Notifier` with `Notify()` and a bounded, non-blocking wake channel.
- `Worker.Run` selects both ticker and wake channel; every wake still calls PostgreSQL `ListDue` and `Claim`.
- `ModelService` accepts an optional notifier and calls it only after `work.Create` succeeds, so failed transactions never trigger a download.

- [x] Add a worker test proving `Notify` causes an immediate scan without waiting for the poll interval.
- [x] Add a service test proving notification occurs after task creation.
- [x] Run focused tests, observe the expected failures, implement the notifier, and wire the same worker instance into `ModelService` and `WorkerSupervisor`.
- [x] Run focused tests and verify idempotent replay remains on the existing PostgreSQL path.

### Task 3: Ensure tenant bucket immediately before provider download

**Files:**
- Modify: `internal/worker/import_executor.go`
- Modify: `internal/worker/import_executor_test.go`
- Modify: `cmd/ani-model-service/main.go`
- Modify: `docs/execution/status.md`
- Create: `docs/execution/records/2026-09-16-tenant-bucket-import-job.md`

**Interfaces:**
- `ImportExecutor` checks `BucketManager.EnsureBucket` before `FetchContent`; any provider or bucket error follows the existing retry/terminal-failure path.
- Existing structured lifecycle logs include `bucket ensure started`, `bucket ensured`, and `bucket ensure failed` with tenant/task fields.

- [x] Add a fake bucket manager and test that download is not called when ensure fails, and is called once for an existing bucket.
- [x] Run the focused tests and verify the new tests failed before implementation.
- [x] Implement the ensure call and lifecycle logs without logging credentials or URLs.
- [x] Wire the MinIO adapter’s tenant bucket manager into the executor.
- [x] Update the execution record with the exact runtime flow and log commands.

### Task 4: Real verification

**Files:**
- Modify: `docs/execution/status.md`
- Modify: `docs/execution/records/2026-09-16-tenant-bucket-import-job.md`

- [x] Use a new test tenant against the developer Docker PostgreSQL and cluster MinIO.
- [ ] Verify a missing tenant bucket is created once, then a second import sees it as existing.
- [x] Verify real logs show bucket ensure, provider download, upload, checksum, ready, and completion; wake behavior is covered by unit test.
- [x] Verify the task remains recoverable through the PostgreSQL due scan when wake is not delivered (unit coverage; process crash simulation remains pending).
- [x] Run internal/full Go tests, vet, build, Buf lint, module checks, and `git diff --check`.
- [x] Mark only evidenced behavior `pass`; leave formal Gateway/IAM and crash-restart integration `not_verified`.
