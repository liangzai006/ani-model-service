# Progress

## 2026-09-18 continuation
- Resumed from session `01a0a825-88b6-7070-9dbb-dc0431c4d6a4` and applied the preserved Inference reference test patch to the actual `/root/kubercon/ani-inference-service` checkout.
- Model host acceptance passed: `TestProtectedDeletionPostgres`, both deletion lock races, and pagination integration tests; `make verify` passed.
- Inference host acceptance passed: `TestModelReferencesGRPCPostgres`, `go test -p 2 ./... -count=1`, `go vet -p 2 ./...`, and `go build -p 2 ./...`.
- B is complete. C remains the next slice: complete model import, artifact organization, task query, and retry behavior.
- C implementation is now in place: `GetImportTask`/`RetryImportTask`, PostgreSQL task read/retry, provider manifest listing, and safe repository tar packaging. Focused tests, real PostgreSQL task retry, provider manifest endpoint read, and Model `make verify` passed.
- C real acceptance is now complete: `TestImportManifestRepositoryE2E` used a random tenant with real HuggingFace `sshleifer/tiny-gpt2@main`, produced a 9-entry 4,742,656-byte tar, verified download checksum/artifact/ready, queried the completed task, reset it through `RetryImportTask`, and processed it to completed again. Evidence: `docs/execution/records/2026-09-18-model-manifest-import.md`.
- D version-switch slice is implemented in the actual Inference checkout: Update accepts `model_version_id`, revalidates the target ready version through Model gRPC, and writes its snapshot into the next generation. Generic engine, positive Deployment scale, one-worker distributed scale, and artifact-sized PVC materialization are now implemented; focused and full Inference test/vet/build gates passed. Real Kubernetes lifecycle/recovery acceptance remains open.

## 2026-09-17 AI service prototype fixes
- Subsequent authorization restored host access: A pagination test passed on
  real recycling PostgreSQL (rollback fixture); host full tests and make verify
  passed before starting the deletion changes.
- B now implements real version-reference gRPC/query/wiring in Inference and
  guarded model/version soft deletion with shared/exclusive row locks in Model.
  Unit/race checks and Model vet/build pass. B database/concurrency validation
  remains pending after automatic approval capacity failures returned.
- Pending Inference test patch is explicitly preserved in
  docs/execution/patches/2026-09-17-inference-reference-tests.patch; it has NOT
  been applied to the actual Inference repository. Resume from
  docs/execution/records/2026-09-17-model-deletion-references.md.
- Implemented Model filtering, stable cursor pagination, version pagination,
  latest-version bulk lookup and persisted metadata mapping in the real checkout.
- Observed RED then GREEN for pagination truncation and version metadata tests.
- Service/Postgres non-DB tests, all-package vet/build and deterministic sqlc
  generation passed. PostgreSQL tests remain skipped without MODEL_DATABASE_URL.
- Full tests hit sandbox TCP listener restrictions; make verify hit offline
  pinned-generator module resolution. Host execution repeatedly rejected by the
  automatic reviewer because its model is at capacity, including read-only
  Docker inspection. No host command was bypassed.
- Resume with transaction-rollback TestCatalogPaginationPostgres and host make
  verify. Details: docs/execution/records/2026-09-17-model-list-pagination.md.
- AI functionality is not complete; reference-guard deletion and subsequent
  slices remain in docs/superpowers/plans/2026-09-17-ai-services-functional-completion.md.

## 2026-09-16 live goal
- Restored actual worktrees and user-authorized scope.
- Host automatic review eventually recovered. Applied eight-file corrected
  Model integration patch to actual ani-inference-service, no commit/push.
- Host `go test ./internal/data/model ./cmd/ani-inference-service -count=1`
  passed in the actual Inference repository.
- Read current cluster nodes, GPU allocation, image caches, storage classes and
  correct Inference CRD name. No cluster mutation yet.
- Chose minimal full-model bundle via existing upload confirmation API and
  PVC/Job materialization matching the existing durable runner sequence.

- Full pinned SmolLM2-135M archive prepared (272455680 bytes), SHA256
  38f8fe399fa9017f1a8278c4627f328f33c5743cd3e5dda8518cbae8a876bc62.
- Model loopback instance 19192 started with explicit tenant-bound bearer resolver.
  Fixed invalid 90s config to allowed 30s gRPC timeout before startup.
- Uploaded through GetUploadURL + PUT + CreateModelVersion; actual ready version
  d8ebd3cc-efd7-414d-803d-345df4af34a0, model b344eda0-13ea-47fa-bc7a-1c30453f05d1,
  tenant 99999999-9999-4999-8999-999999999993, version 12fd25f77366-live1.
- Existing Inference migrations 000001..000012 applied atomically to recycling
  after proving no Inference tables existed; Model tables left unchanged.
- Materialization Job/PVC, safe tar/SHA checking, readonly/offline runtime mount,
  native development quota/publication, actual completion probe, scoped identity
  and single-tenant worker implemented. Focused non-network tests passed.
- 28-file runtime patch applied to ACTUAL Inference checkout after git apply
  --check. Earlier malformed new-file headers were fixed before application.
- Host full Inference tests and dedicated namespace creation currently running.
  Automatic approval capacity failures are intermittent; successful retries
  used the original authorized scope, without bypassing sandbox restrictions.

## Final live evidence
- Model gRPC upload returned ModelVersion `d8ebd3cc-efd7-414d-803d-345df4af34a0`
  (`smollm2-135m-instruct`, `12fd25f77366-live1`) with artifact ref
  `99999999-9999-4999-8999-999999999993/smollm2-135m-instruct/12fd25f77366-live1/model.tar`,
  272455680 bytes, SHA256
  `38f8fe399fa9017f1a8278c4627f328f33c5743cd3e5dda8518cbae8a876bc62`.
- Final Inference service ID `0f0265c3-9d18-410f-9a1b-851101754799`, operation
  `34909728-bfff-41d4-bded-da38d7a49995`, name `smollm2-live-20260916-final`.
  Namespace `ani-model-inference-e2e-retry-20260916`; Deployment and published
  NodePort Service are `smollm2-live-20260916-final` and
  `smollm2-live-20260916-final-published`, NodePort `30325`.
- PVC `ani-model-3d79e1f0e6ef9a8a82d99df1` is Bound on CephFS; Job
  `ani-model-3d79e1f0e6ef9a8a82d99df1-fetch` is Complete 1/1 and logged the exact
  SHA256 and 272455680 bytes. Deployment is 1/1 Ready on the selected GPU node.
- Durable API readback is `runtime_phase=ready`, `publication_phase=published`,
  `invocation_health=healthy`, operation `succeeded/complete` with completion
  timestamp 2026-09-16T11:29:48Z.
- Real request: `curl --fail -H 'Content-Type: application/json' -d
  '{"model":"smollm2-135m","prompt":"The capital of France is","max_tokens":8,"temperature":0}'
  http://10.10.1.67:30325/v1/completions`; response text was
  ` Paris. Paris is the largest city in`.
- Negative evidence: Model/Inference cross-tenant credential returned
  PermissionDenied, nonexistent model version returned NotFound, and the first
  occupied-GPU launch remained degraded with a bounded vLLM cache-memory error.
  The successful retry was created through gRPC with a new service ID; no DB
  status was edited. All Inference tests after the final code changes passed.
