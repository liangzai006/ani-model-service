# Model Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver an independent Model service that preserves the legacy model gRPC contract while providing durable model versions, artifacts, imports, download authorization, and runtime defaults for Inference.

**Architecture:** A Kratos gRPC service owns PostgreSQL tables in the `public` schema and uses sqlc for all queries. Durable import work is claimed from PostgreSQL with lease/CAS fencing; storage and IAM are external adapters. Inference consumes `GetModelVersion` and `GetModelDownloadURL` and never writes Model data.

**Tech Stack:** ani-kratos-layout SHA `fd18422211c741dd5242d2992c3d307d522aa28c`, Go, Kratos gRPC transport, PostgreSQL, sqlc, pgx, Buf, structured observability.

## Global Constraints

- Public business flow is client HTTP → independent Gateway → Model gRPC. This
  repository owns no Gateway or business HTTP routes; admin HTTP exposes probes
  and metrics only. External Storage operations require its confirmed contract.
- Independent repository `/root/kubercon/ani-model-service`; do not modify `/root/kubercon/ANI`.
- PostgreSQL uses explicit `tenant_id` predicates and no RLS; Model temporarily uses the same development database credentials as Inference while schema and write boundaries remain separate.
- Storage, IAM, quota, Kubernetes and Inference are external domains; no shared write tables, cross-service foreign keys or copied authority tables.
- No fixed model, fixed provider or development profile in production; missing required providers keep `/readyz` false.
- All writes, idempotency results, audit events and durable work state commit in local transactions.

## File Map

- `api/model/v1/public.proto`, `api/model/v1/conf.proto`: versioned public and internal gRPC contracts.
- `migrations/000001_model_control_plane.sql`: authoritative Model schema and constraints.
- `queries/model.sql`, `queries/work.sql`: sole SQL query sources for sqlc generation.
- `internal/biz/model`: model/version/artifact state machine and ports.
- `internal/biz/work`: durable import work lease and retry logic.
- `internal/data/postgres`: sqlc adapters and local transaction boundaries.
- `internal/data/storage`: Storage adapter interface and signed URL implementation boundary.
- `internal/service`: gRPC validation, tenant/actor context checks and use cases.
- `cmd/ani-model-service`: Kratos lifecycle, readiness and worker wiring.
- `docs/specs`, `docs/adr`, `docs/execution`: service-owned contracts and evidence.

### Task 1: Generate the independent Kratos repository

**Files:** create the layout tree under `/root/kubercon/ani-model-service`; create `docs/execution/records/2026-09-14-layout-baseline.md`.

- [ ] Record the exact layout repository, commit SHA, generator command, generated tree and allowed modifications.
- [ ] Generate the Go module and Kratos gRPC/health/admin lifecycle using the same template contract as Inference.
- [ ] Run `go test ./...`, `go vet ./...`, `go build ./...` and Buf lint on the empty generated service.

### Task 2: Define the compatible Model gRPC contract

**Files:** `api/model/v1/public.proto`, `api/model/v1/conf.proto`, generated `*.pb.go`, `docs/specs/model-service.md`.

- [ ] Copy only the public field and method semantics from the legacy read-only prototype into a new package; do not copy implementation code.
- [ ] Keep `GetModelVersion` and `GetModelDownloadURL` request/response compatibility and add optional `engine_type`, `startup_command`, `startup_args` fields with new field numbers.
- [ ] Define trusted actor/workload metadata and tenant cross-check rules; reject missing trusted identity with `Unauthenticated`.
- [ ] Generate code with Buf and add validation tests for IDs, checksum, status and idempotency keys.

### Task 3: Create Model PostgreSQL schema and sqlc layer

**Files:** `migrations/000001_model_control_plane.sql`, `queries/model.sql`, `queries/work.sql`, `sqlc.yaml`, `internal/data/postgres/*`.

- [ ] Add `public.models`, `public.model_versions`, `public.model_artifacts`, `public.model_import_tasks`, and audit/work lease tables with tenant-scoped keys and checks.
- [ ] Add immutable artifact checksum/reference constraints and soft-delete/reference protection fields.
- [ ] Add queries for CRUD, version lookup, ready validation, keyset lists, idempotency replay, task claim/renew/retry and audit insertion; every tenant query includes `tenant_id`.
- [ ] Run sqlc generation and real PostgreSQL tests covering cross-tenant negative access and duplicate idempotency keys.

### Task 4: Implement model/version/artifact use cases

**Files:** `internal/biz/model/*`, `internal/data/postgres/model_store.go`, `internal/service/public.go` and tests.

- [ ] Implement Create/Get/List/Delete model and Create/Get/List version transactions.
- [ ] Enforce `ready` plus complete artifact checksum before returning a deployment-usable version to Inference.
- [ ] Persist audit event fields: tenant, actor, workload, request ID, operation/task ID, action, before/after state and error class.
- [ ] Test tenant isolation, soft delete, immutable artifact facts, status transitions and replay behavior.

### Task 5: Implement storage and signed download adapters

**Files:** `internal/biz/storage`, `internal/data/storage/*`, `internal/service/model_download.go` and tests.

- [ ] Define an external Storage port for upload URL, object existence, checksum verification and signed download URL.
- [ ] Implement the configured Storage adapter; do not create buckets, PVCs or storage lifecycle resources owned by Storage service.
- [ ] Enforce short-lived download URLs, requester identity, model-version readiness and tenant scope.
- [ ] Test expiry, authorization, checksum mismatch and provider error retry classification with explicit fakes only in isolated tests.

### Task 6: Implement durable import tasks and worker recovery

**Files:** `internal/biz/work/*`, `internal/data/postgres/work_store.go`, `internal/data/importer/*`, worker wiring and tests.

- [ ] Implement upload/import task state machine `pending → importing → ready/error` with lease token, attempt count, due time and CAS fencing.
- [ ] Add source adapters for upload, HuggingFace and ModelScope behind interfaces; persist remote download, object write and checksum outcomes.
- [ ] Recover due tasks after process restart and reject stale worker results after lease expiry or version replacement.
- [ ] Add real PostgreSQL crash/restart simulation tests for claim, retry, timeout and idempotent completion.

### Task 7: Wire Kratos service, readiness and observability

**Files:** `cmd/ani-model-service/main.go`, `app.go`, config, `internal/server/*`, metrics and docs.

- [ ] Wire PostgreSQL, Storage, IAM and import worker dependencies through explicit constructors; no hidden default provider.
- [ ] Keep `/healthz` process-level and make `/readyz` require PostgreSQL, configured Storage and a live durable worker.
- [ ] Add bounded metrics for task backlog, oldest age, claims, retries, checksum failures and provider error classes.
- [ ] Test graceful shutdown, readiness failure when dependencies are absent and worker recovery on startup.

### Task 8: Integrate Inference through the formal Model client

**Files:** In the Model repo, `internal/client/inference.go` and contract tests; in Inference repo, Model adapter/config only.

- [ ] Generate a versioned Model client from the new Proto and implement `ModelPort.EnsureModel` using `GetModelVersion`.
- [ ] Map `storage_path/checksum_sha256` to Inference artifact snapshot and map runtime defaults to `engine_runtime/command_argv`.
- [ ] Reject non-ready versions and preserve provider error/not-found semantics for durable retry.
- [ ] Run an isolated two-process gRPC integration test with real PostgreSQL Model data and Inference PostgreSQL data.

### Task 9: Real dependency verification

**Files:** `docs/execution/status.md`, `docs/execution/records/*`.

- [ ] Apply Model migrations to an isolated database/schema using separate credentials.
- [ ] Verify upload/import/download against the configured Storage service and verify checksum and URL expiry.
- [ ] Verify IAM actor/workload propagation and cross-tenant denial through the real gRPC path.
- [ ] Verify Inference deployment using a real ready Model version; keep any missing external dependency explicitly `not_verified`.
