# Inference Accelerator and KServe Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the Accelerator GPU request/plan contract to Inference and apply a frozen plan to KServe Deployment/LWS workloads without changing engine command arguments or touching the old environment.

**Architecture:** Inference accepts a nested `resource.gpu` request that follows Accelerator's `GpuRequest`, persists it with the generation, resolves a frozen plan through an explicit Accelerator adapter, and passes that plan to KServe renderers. CPU/memory remain Kubernetes resource maps. Higress and ani-governance are sibling northbound services: Higress is the inference data-plane gateway, while Governance is the business/control-plane gateway for Model and Inference management APIs. Governance quota-owner wiring remains a separate follow-up.

**Tech Stack:** Go 1.26.7, Protocol Buffers/Buf, PostgreSQL/sqlc, controller-runtime, KServe 0.15/0.16 CRDs, native LWS, Accelerator `v1.2` protobuf client.

**Current implementation status (2026-09-27):** The nested request, immutable
plan persistence, optional resolver, durable resolve gate and KServe/LWS
projection are implemented and verified with local tests. The new cluster has
no GPU capacity, so real scheduling remains unverified. The Accelerator server
still grants `ResolveGpuRequest` to Governance rather than the Inference mTLS
identity; no upstream authorization change was made, so enabling the resolver
in production remains a separate activation step.

## Global Constraints

- Do not modify, apply to, or delete anything in the old cluster.
- Do not commit changes in this session; leave all edits reviewable in the working trees.
- Do not synthesize, append, normalize or otherwise change engine startup command/args.
- `resource.gpu.pool_id` follows the Accelerator contract; do not persist a parallel `pool` field.
- Reject legacy GPU map entries; CPU and memory remain in `requests`/`limits`.
- Omitted `resource.gpu` means no GPU resolution or GPU scheduling, with no automatic engine or resource fallback.
- No Governance quota owner, IAM, public TLS, Notebook, Dashboard, Trainer or unrelated Kubeflow work.
- Do not add a speculative Volcano PodGroup CRD; current Accelerator contracts say the owner maps an existing managed PodGroup.
- Do not claim real GPU validation while the new cluster has no GPU/device-plugin capacity.
- Every production-code change gets a focused test written and observed failing before implementation.

---

### Task 1: Add the nested GPU request contract and domain validation

**Files:**
- Modify: `/root/kubercon/ani-inference-service/api/inference/v1/inference.proto`
- Generate: `/root/kubercon/ani-inference-service/api/inference/v1/inference.pb.go`
- Create/modify: `/root/kubercon/ani-inference-service/internal/biz/gpu/`
- Modify: `/root/kubercon/ani-inference-service/internal/biz/resources/resources.go`
- Modify: `/root/kubercon/ani-inference-service/internal/service/inference.go`
- Test: `/root/kubercon/ani-inference-service/internal/biz/gpu/gpu_test.go`, `internal/service/inference_test.go`, `internal/biz/resources/resources_test.go`

**Interfaces:**
- Produces `inference.v1.GpuResource` with `cluster_id`, `pool_id`, `profile_id`, `profile_version`, `replicas`, `devices_per_replica`, and `container_name`.
- Produces transport-neutral GPU request/plan types and validation helpers consumed by persistence and Kubernetes adapters.
- Extends `ResourceSpec` and normalized resources with an optional GPU request while preserving CPU/memory map behavior.

- [ ] **Step 1: Write failing tests** for valid nested GPU input, missing profile fields, invalid UUID/version/counts, legacy GPU map rejection and Deployment/LWS topology counts.
- [ ] **Step 2: Run focused tests** with `go test ./internal/biz/gpu ./internal/biz/resources ./internal/service`; confirm failures are due to missing nested GPU behavior.
- [ ] **Step 3: Add the proto message and domain types** without importing protobuf/Kubernetes into the domain package. Validate Accelerator's `replicas` 1..16, `devices_per_replica=1`, valid container name and profile version.
- [ ] **Step 4: Regenerate protobuf output** using the repository's Buf 1.50 workflow (`buf generate --template buf.gen.yaml --path api/inference/v1/inference.proto`) and do not hand-edit generated files.
- [ ] **Step 5: Map Create/Update requests** into the domain request, reject legacy GPU map entries, and preserve the nested request in the deterministic idempotency hash.
- [ ] **Step 6: Run focused tests** and `buf lint`; expected result is PASS.

### Task 2: Persist GPU requests and immutable resolved plans by generation

**Files:**
- Create: `/root/kubercon/ani-inference-service/migrations/000016_inference_gpu_plan.sql`
- Modify: `/root/kubercon/ani-inference-service/queries/inference.sql`
- Generate: `/root/kubercon/ani-inference-service/internal/data/postgres/inference.sql.go`, `models.go` as required by sqlc
- Modify: `/root/kubercon/ani-inference-service/internal/data/postgres/create_usecase.go`, `update_usecase.go`, `runtime_source.go`, `read_usecase.go`, `operation_store.go`
- Modify: `/root/kubercon/ani-inference-service/internal/biz/inference/create.go`, `update.go`, `runner.go`
- Test: corresponding PostgreSQL/use-case tests and migration replay test

**Interfaces:**
- Persists the caller GPU request and nullable frozen `ResolvedGpuPlan` JSON/digest in each immutable inference generation.
- `RuntimeSource.CurrentRuntime` returns the plan to Kubernetes rendering and preserves it across retries.

- [ ] **Step 1: Write failing persistence tests** that create a GPU generation, read back the request, store a plan once, reject a changed plan digest for the same generation, and clone it unchanged on Update/Start.
- [ ] **Step 2: Run focused PostgreSQL tests** and confirm the new columns/query fields are missing.
- [ ] **Step 3: Add one forward-only migration** with JSONB request/plan columns and a digest column; do not alter earlier migrations or start-up migration behavior.
- [ ] **Step 4: Extend the single sqlc query source** for insert, clone, read and runtime projection; run `sqlc generate`.
- [ ] **Step 5: Thread GPU request/plan through CreateInput, UpdateInput and DesiredRuntime** while keeping CPU-only rows backward-compatible with NULL/empty GPU values.
- [ ] **Step 6: Run focused tests, `sqlc generate` again for determinism, and `git diff --check`; expected result is PASS.

### Task 3: Add the Accelerator client and durable resolve step

**Files:**
- Modify: `/root/kubercon/ani-inference-service/go.mod`, `/root/kubercon/ani-inference-service/go.sum`
- Create: `/root/kubercon/ani-inference-service/internal/data/accelerator/client.go`, `plan_adapter.go`
- Modify: `/root/kubercon/ani-inference-service/internal/biz/inference/runner.go`, operation step definitions and stores
- Modify: `/root/kubercon/ani-inference-service/cmd/ani-inference-service/main.go`
- Test: `internal/data/accelerator/*_test.go`, runner resolve-step tests, composition tests

**Interfaces:**
- `AcceleratorPort.Resolve(context.Context, gpu.Request) (gpu.Plan, error)`.
- The client uses the pinned Accelerator module at commit `1d32dd9a9173b8869fa0ef2ae64e20b88f2ca0a3` and maps no unknown fields.
- The resolve step runs before materialization/runtime apply and persists one immutable plan per generation.

- [ ] **Step 1: Resolve the authorization gap before enabling production calls**: the current Accelerator server permits Governance to call `ResolveGpuRequest` but not Inference. This implementation leaves the upstream authorization unchanged; the adapter is opt-in and a supplied GPU request fails closed until the least-privilege Inference grant is approved and deployed.
- [ ] **Step 2: Write failing fake-client tests** for exact request mapping, plan persistence, retry reuse, resolution errors and configured-cluster mismatch.
- [ ] **Step 3: Implement the client with the upstream generated gRPC types**; do not copy the upstream `internal` package, call arbitrary URLs, or add plaintext fallback.
- [ ] **Step 4: Add a durable resolve step** that never re-resolves when a plan digest already exists and never creates KServe objects before successful resolution.
- [ ] **Step 5: Wire configuration for the new cluster only**; reject a request whose `cluster_id` does not match the configured cluster identity.
- [ ] **Step 6: Run adapter/runner/composition tests; expected result is PASS or an explicit blocked result if the upstream authorization decision is not approved.

### Task 4: Apply the frozen plan to KServe Deployment and LWS renderers

**Files:**
- Modify: `/root/kubercon/ani-inference-service/internal/data/kubernetes/renderer.go`
- Modify: `/root/kubercon/ani-inference-service/internal/data/kubernetes/kserve.go`
- Modify: `/root/kubercon/ani-inference-service/internal/data/kubernetes/executor.go`
- Test: renderer, KServe and runtime-observation tests

**Interfaces:**
- `RuntimeSpec` carries the immutable `gpu.Plan`.
- Both KServe templates apply the plan's scheduler, managed node labels, pod annotations, runtime class and per-container GPU limits.

- [ ] **Step 1: Write failing renderer tests** for Deployment and LWS plan application, target-container matching, invalid/missing plan rejection and unchanged command/args.
- [ ] **Step 2: Run focused Kubernetes tests** and confirm the plan fields are currently absent from rendered objects.
- [ ] **Step 3: Apply plan fields exactly** to the GPU-bearing container and both LWS leader/worker templates; keep existing autoscaler, model mount, endpoint and CPU behavior unchanged.
- [ ] **Step 4: Reject a supplied `resource.gpu` request without a valid frozen plan**. When `resource.gpu` is omitted, render no GPU placement, selector, queue or runtime-class fields; do not infer a CPU engine mode or synthesize any engine flags.
- [ ] **Step 5: Update readiness matching** so expected Accelerator-owned fields are checked while KServe-injected fields remain accepted.
- [ ] **Step 6: Run focused renderer/KServe tests and full `go test ./...` when the dependent API/persistence tasks are green.

### Task 5: Validate locally without cluster mutation

**Files:**
- Modify: `/root/kubercon/ani-model-service/docs/execution/status.md` and a new execution record only after verification
- Test artifacts: no production files

- [ ] **Step 1: Run `gofmt` on changed Go files, `buf lint`, `buf generate`, and `sqlc generate`.
- [ ] **Step 2: Run `go test ./... -count=1`, `go vet ./...`, `go build ./...`, and `git diff --check` in the Inference repository.
- [ ] **Step 3: Run model-service client tests if generated Inference API changes affect its client; do not change old deployment manifests.
- [ ] **Step 4: Record that real GPU/KServe scheduling remains `not_verified` because the new cluster has no GPU/device plugin, and record any Accelerator authorization blocker precisely.
- [ ] **Step 5: Leave all code and documentation changes uncommitted for user review.
