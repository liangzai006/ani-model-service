# Progress

## 2026-09-21 new-project image labels and gateway scope

- User reiterated that old projects/resources/images must remain untouched;
  trusted IAM and quota integration are deferred.
- Added `ani-model-new-20260921-r1` to the two new-project images, preserving
  the Model `63c3234b...` and Import `c8283c4e...` digests. Applied tag+digest
  references only to the new `ani-model` Deployment; rollout returned 1/1 Ready
  with zero restarts on the new Pod.
- Read-only new-cluster checks found Gateway API CRDs and APISIX absent. No
  gateway installed yet. Its basic routing infrastructure can be installed
  independently of the deferred IAM/quota integration.

## 2026-09-21 new-cluster preflight

- Read `.clusterenv` and confirmed the new environment is
  `192.168.102.68/72/73`; the existing local kubeconfig remains pointed at the
  old `10.10.1.x` cluster.
- Used only read-only SSH/kubectl commands through the new control plane's
  `/etc/kubernetes/admin.conf`. No resource was applied, deleted, or edited in
  either environment.
- New cluster is Kubernetes v1.37.0 with three Ready nodes and fresh Rook
  CephFS/RBD. Ceph is Ready with an insecure-key-type warning; no S3 object
  store, PostgreSQL, MinIO, application namespace, gateway, GPU, LWS or KServe
  is present.
- Recorded the required Model-only and full Inference prerequisites in
  `docs/execution/records/2026-09-21-new-cluster-preflight.md`. Deployment is
  intentionally pending an explicit new-cluster-only resource plan.

## 2026-09-21 foundation installation

- User narrowed the first deployment to PostgreSQL and MinIO only; E2E
  namespaces, image registry, TLS/IAM, gateway, GPU and Kubeflow are deferred.
- Created only new-cluster namespace `ani-foundation` and applied the
  reproducible manifest `docs/deploy/foundation/postgres-minio.yaml`.
- PostgreSQL is Ready on a 20Gi `rook-ceph-block` PVC and MinIO is Ready on a
  100Gi `rook-ceph-block` PVC. Services are ClusterIP-only.
- Verified PostgreSQL `pg_isready`, in-cluster DNS for both Services, MinIO
  health endpoint, and creation of bucket `ani-models`.
- Initial PostgreSQL UID/volume permission failure was fixed with UID/GID 26
  securityContext; the PVC was preserved. Full evidence is in
  `docs/execution/records/2026-09-21-foundation-install.md`.

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

## 2026-09-21 new-cluster foundation

- Installed only the new-cluster `ani-foundation` PostgreSQL and MinIO
  workloads on Rook Ceph RBD. PostgreSQL is Ready with a 20Gi PVC; MinIO is
  Ready with a 100Gi PVC and the `ani-models` bucket.
- Applied Model migrations `000001` through `000006` to the new `ani_model`
  database. The one-shot Job is `Complete` and independently verified five
  expected public tables plus `models.model_id`.
- Fixed the migration Job's ConfigMap traversal after observing that projected
  files are symlinks. The corrected manifest accepts regular files and
  symlinks and reports the exact expected table count.
- The old local kubeconfig context remains `kubernetes-admin@kubernetes`; no
  old-cluster mutation was performed. Inference database/schema, Model/Inference
  services, TLS/IAM, registry, gateway, GPU, and Kubeflow remain pending.

## 2026-09-21 new-cluster Model deployment

- Created only the dedicated `ani-model` namespace, ServiceAccounts, and
  namespace-scoped import-controller RBAC. Database and MinIO credentials were
  copied into namespace-local Secrets without exposing their values.
- Published the current repository's Model service and import Job images to
  the existing reachable Harbor registry by immutable digest. Both workers
  pulled the import image successfully.
- Deployed the Model service with PostgreSQL/MinIO dependencies and
  `ANI_MINIO_TENANT_BUCKETS=false`; the Pod is `1/1 Ready`, and admin health
  and readiness both return 200.
- The gRPC business path reaches the refactored service but returns
  `Unauthenticated: trusted principal is required`, as expected while TLS/IAM
  remains deferred. A real import is therefore intentionally pending.
- A same-named Harbor image was found to be an unrelated
  `github.com/kubercloud/ani/services/model-service` binary; its failed new
  Deployment was removed before the correct image was built and applied.

## 2026-09-21 new-cluster gateway

- Installed Gateway API standard CRDs v1.6.1 and APISIX Helm chart 2.17.0 in
  the new cluster's isolated `ingress-apisix` namespace. The legacy APISIX
  values and old E2E route were not applied.
- Mirrored APISIX, Ingress Controller, ADC, etcd and busybox into new Harbor
  tags `new-cluster-20260921-r1`; the new values file uses only those tags and
  the new cluster's `rook-ceph-block` storage class.
- APISIX, etcd and Ingress Controller are ready. The proxy NodePort is 30090;
  the internal admin Service remains ClusterIP and is protected by a
  namespace-local randomly generated Secret.
- `GatewayClass/ani-apisix` and `Gateway/ingress-apisix/ani-apisix` are
  Accepted and Programmed. No HTTPRoute was created because the new-cluster
  Inference backend is not deployed yet; an unconfigured proxy returns the
  expected HTTP 404.
- IAM/trusted principal, quota, TLS, GPU, Kubeflow and the Inference backend
  remain deferred. Model/Foundation readiness was rechecked after installation.

## 2026-09-22 new-cluster Inference control plane

- Created the isolated `ani_inference` database in the new cluster's existing
  PostgreSQL instance and copied only the new database credentials into the
  `ani-inference` namespace. The old environment and its databases were not
  touched.
- Applied all twelve Inference migrations with the one-shot
  `inference-migrations-20260922` Job. The final independent query found 11
  `inference_%` tables.
- Built and published the refactored Inference control-plane image as
  `ani-inference-new-20260922-r3` at digest
  `sha256:eba3ec6edb4a8d3772631293582106e6b3ab9816573db95bdfac6213ef159c08`.
  The image includes the non-background readiness fix and a readable runtime
  config for its non-root user.
- Deployed one replica with internal ClusterIP gRPC/admin Services and
  `ANI_KUBERNETES_ENABLED=false`. The Pod is `1/1 Running`; `/healthz` and
  `/readyz` both return `{"status":"ok"}` and `{"status":"ready"}`.
- This is the database-backed control plane only. IAM/trusted principal,
  Model gRPC TLS, Kubernetes runtime reconciliation, quota/publication,
  materialization, GPU capacity, and an external HTTPRoute remain deferred.

## 2026-09-23 real Model-to-Inference validation

- Model remote HF Job reached the expected egress blocker; no status was forged.
- Downloaded pinned SmolLM2-135M on the development machine and finalized a
  ready ModelVersion through signed MinIO upload, SHA256 verification and the
  normal Model API.
- Created `smollm2-135m-cpu-real` through Inference gRPC with direct tenant
  metadata. Materializer Job/PVC completed on CephFS, vLLM CPU became ready,
  HTTPRoute was Accepted/ResolvedRefs, and APISIX returned HTTP 200 with a
  non-empty completion.
- Updated deployed Inference to r11 and recorded tar normalization plus
  previous-generation binding fixes. A separate update-lifecycle stale
  generation issue remains for follow-up.

## 2026-09-23 verification recheck

- Fresh new-cluster call returned HTTP 200 from APISIX `/v1/completions` with non-empty CPU vLLM output; ModelVersion is ready and create operation is succeeded/complete.
- Fresh full tests passed in both repositories: `go test ./... -count=1` in `ani-model-service` and `ani-inference-service`.
- Materializer boundary passed with a temporary HTTP-served 513.0 MiB tar; it was removed after SHA256/download/extraction verification.
- Remaining status is unchanged: direct HF import is blocked by new-node egress, and a separate update test is stuck in stale-generation retry.

## 2026-09-23 larger model real validation

- [x] Validate a model materially larger than the 272MB SmolLM2 archive through Model upload, checksum/size persistence, CephFS materialization, vLLM startup, APISIX publication and a real completion.
- [x] Verify Qwen2.5-1.5B (3,098,992,640 bytes; SHA256 `d0f3c1294ac34d9ca84858cf93bfe4dbae8ebdfa2a317d1f90970b1dd85f11dd`) with a 6Gi PVC and 3 CPU/6Gi request runtime.
- [x] Fix and regression-test missing ModelVersion `SizeBytes` persistence discovered by the larger archive.
- [ ] Add DNS-1035 validation/normalization for Inference endpoint names and define multi-model `/v1` route selection.
- [ ] Validate 3B/7B only after a worker with enough memory or GPU is available; current new cluster has no GPU and is capacity-constrained.
