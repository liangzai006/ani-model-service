# Model → Inference → Kubernetes → real invocation

Acceptance: a Model-owned full artifact is obtained over Model gRPC, checksum
verified into a PVC by Inference, mounted into a ready runtime, and produces a
real completion. Persisted operation/service projections must agree. No commits,
push, legacy ANI edits, or adoption/deletion of unrelated cluster resources.

## Phases
- [x] Recover worktrees and apply the previously validated Model client patch.
- [x] Inspect current cluster GPU/storage/images and existing source boundaries.
- [x] Prepare a complete pinned small-model archive and import through Model RPC.
- [x] Add real PVC/Job materialization, runtime mount and engine probes.
- [x] Add opt-in restricted development identity and native Kubernetes quota/publication adapters; wire actual composition roots.
- [x] Verify local tests and generation gates, then run actual service processes and create through Inference RPC.
- [x] Verify ready workload, successful real completion, durable state, idempotency, and negative paths; leave successful resources available.
- [x] Record exact identities, digest, commands, results and deferred production dependencies.

## Decisions
- Existing Model upload URL + CreateModelVersion confirmation supports one
  archive; reuse that API rather than add a second import orchestrator.
- Model format remains the underlying weights format (`safetensors`); the
  immutable artifact ref ends in `.tar` and the materializer accepts only the
  documented safe tar bundle contract for this slice.
- Use SmolLM2-135M-Instruct, pinned upstream commit, minimal runtime files,
  no remote code or network fallback inside vLLM. Cached vLLM image digest.
- PVC + Job precedes runtime apply, matching existing materialize_model step.
  Signed URL is transient in a narrowly owned Kubernetes Secret, never in the
  Inference spec, command arguments or logs. Mark materialized only from actual
  successful Job condition; runtime ready additionally needs engine health.
- Development mode defaults off, requires a dedicated tenant and namespace,
  strong bearer token and loopback control-plane RPC addresses. Namespace-local
  ResourceQuota is the explicitly scoped development authority (not production
  billing/capacity reservation). Publication uses a real owned NodePort Service
  and ready EndpointSlice confirmation, with an actual invocation check.
- Production IAM/Gateway and quota billing remain deferred. Default mode keeps
  existing provider readiness gates. Development mode is create-only.

## Errors / resolved blockers
- Sandbox cannot run snap kubectl or bind TCP listeners; host escalation required.
- Host approval timeout retried, then host access recovered; patch now applied.
- CRD guessed as inference.ani.io was wrong; actual existing group is
  ani.kubercloud.com, confirmed by source and cluster listing.
- recycling contains Model tables only. Verify existing ani_inference database
  before choosing DSNs; do not create/alter tables speculatively.
- The new cluster has no default StorageClass; only `local-path` exists. The
  real run used an explicit Helm override, and the default Inference source
  still falls back to absent `cephfs` when the value is blank.
- The clean Inference chart reused a previously stored map-shaped
  `migration.image` value while the default template expects a string, causing
  one `InvalidImageName` hook. The failed hook was deleted and the next Helm
  upgrade supplied the string explicitly.
- Governance platform login and tenant creation succeeded, but the temporary
  tenant received HTTP 403 for Model creation. The deployed route prefix and
  role permission grants do not match the checked-in Governance contract;
  Governance was intentionally not changed.

## New cluster audit (2026-09-21)

- [x] Keep the old `kubernetes-admin@kubernetes` environment read-only and
  identify the `.clusterenv` control plane.
- [x] Run a read-only preflight against the new Kubernetes/Ceph cluster.
- [x] Record missing PostgreSQL, S3/MinIO, application, identity, gateway and
  GPU prerequisites without deploying them.
- [x] Deploy and validate only after selecting new-cluster-only namespaces,
  credentials, images and an explicit kubeconfig boundary.

## New-cluster foundation execution (2026-09-21)

Current user constraints: operate only on the new project and `.clusterenv`
cluster. Leave old project resources and image tags untouched. New Model and
import images use `ani-model-new-20260921-r1` plus immutable digests. Trusted
IAM, quota, and the previously deferred TLS integration are not prerequisites
for installing the new cluster's gateway infrastructure. Installing a gateway
does not itself change the Model service's identity enforcement.

- [x] Use the dedicated new-cluster SSH/kubeconfig boundary; leave the old
  context unchanged.
- [x] Create only `ani-foundation`; do not create E2E namespaces yet.
- [x] Deploy single-instance PostgreSQL and MinIO on new-cluster Ceph RBD.
- [x] Verify PostgreSQL readiness, MinIO readiness, cluster DNS and the
  `ani-models` bucket.
- [x] Apply the Model migrations to the new-cluster `ani_model` database.
- [x] Build and deploy the refactored Model service and its Kubernetes import
  Job image in the shared `ani-system` namespace.
- [x] Create the separate `ani_inference` database, apply all twelve
  migrations, and deploy the refactored Inference control plane with its
  Kubernetes runtime composition and namespace-scoped RBAC.
- [x] Install the pinned materializer image reference and CephFS storage class
  configuration; keep actual model materialization pending the deferred Model
  trusted identity/TLS and Quota provider boundaries.
- [x] Install Gateway API v1.6.1 standard CRDs in the new cluster.
- [x] Apply the new-cluster `GatewayClass` and `Gateway`; leave HTTPRoute
  creation pending while identity, Model TLS, and the serving runtime remain
  deferred. Do not apply the legacy E2E route.

## New-cluster runtime wiring (2026-09-22)

- [x] Remove the Inference-only Kubernetes runtime switch. The service now
  requires and starts its Kubernetes runtime in every deployment.
- [x] Install the Inference CRD and LWS controller/CRDs on the new cluster.
- [x] Deploy the new-cluster ServiceAccount, namespace Role and RoleBinding;
  verify the account can manage only the `ani-system` runtime resources.
- [x] Verify the new image starts, opens the gRPC/admin listeners and reaches
  the Kubernetes manager without cache-sync errors.
- [x] Superseded by the current new-cluster deployment: Model business gRPC
  requires the verified mTLS caller identity and tenant metadata. The direct
  access bypass is no longer supported.

## Current new-cluster real validation (2026-09-22)

The previous phases above contain historical old-cluster evidence. This phase
is the active acceptance target and must use only `.clusterenv` at
`192.168.102.68`, with no old-cluster mutation.

- [x] Establish a new-cluster-only port-forward and verify current Model and
  Inference API contracts before creating test resources.
- [x] Import a small real CPU-compatible model into the new Model service;
  verify ready ModelVersion, MinIO object, checksum and size. Remote Hugging Face
  Job egress is a documented environment blocker; signed upload path succeeded.
- [x] Create one Inference service through its gRPC API; verify persisted
  operation, quota reservation, InferenceService CR, PVC and materializer Job.
- [x] Verify the CPU runtime is a real model server, reaches ready state, and
  returns a non-empty response through the new-cluster route.
- [x] Record exact resource names, image digests, request/response evidence,
  and the deferred remote-import/update-lifecycle blockers without changing old resources.

## Current validation follow-ups

- Direct tenant metadata is wired and verified through real gRPC requests.
- New-worker Hugging Face egress prevents direct remote import; the formal
  signed upload path has passed.
- The separate update operation remains pending at observe_absence with stale
  generation; creation and invocation have passed.
- GPU inference has not been validated: the new cluster currently has no GPU.

## Larger model validation (2026-09-23)

- [x] Upload and finalize a pinned Qwen2.5-1.5B archive through the existing Model API; persist its real 3,098,992,640-byte size and checksum.
- [x] Materialize the archive to CephFS, start vLLM CPU with bounded context, and return a real completion through the then-current gateway (now retired).
- [x] Verify the 1.5B path without modifying old-cluster resources; recheck the existing 135M route after the temporary route-isolation check.
- [x] Record current-node capacity and the 3B/7B deferral.
- [ ] Fix endpoint naming for dotted service names and define deterministic multi-model `/v1` publication selection.

## KServe LLMI/LWS CPU validation (2026-09-24)

- [x] Install only the KServe 0.16 LLMI CRDs and controller in the new
  cluster's existing `kubeflow` namespace; keep the existing KServe 0.15
  controller and `inferenceservice-config` unchanged.
- [x] Keep caller-owned engine commands intact; install an empty pipeline
  config so the LLMI controller does not add engine flags.
- [x] Add the LLMI webhook NetworkPolicy required for API Server access and
  verify its webhook configuration can create an LLMI object.
- [x] Create a two-pod pipeline-parallel LWS canary from an RWX model PVC;
  verify the generated LWS, cross-node scheduling and workload Service.
- [ ] Run real multi-node model parallel inference after GPU capacity and
  model-specific distributed startup arguments are available. The current
  CPU canary only validates KServe/LWS orchestration.

## Real API lifecycle validation (2026-09-24)

- [x] Apply the LLMI binding migration in the new cluster database only.
- [x] Deploy the static Inference image with the LWS observer fix.
- [x] Create and delete a two-pod LWS through the real Inference gRPC API.
- [x] Verify KServe readiness, cross-node Pods, generated Service endpoints,
  HTTPRoute status and a real Higress NodePort request.
- [x] Remove the API-created test PVCs, Jobs, Secrets and Pods by exact owner
  label; retain the existing 135M baseline service.
- [ ] Run distributed model inference after GPU capacity and model-specific
  parallel startup configuration are available.

## Governance / Accelerator integration review (2026-09-25)

- [ ] Inspect current upstream Governance API and domain-client wiring.
- [ ] Inspect current upstream Accelerator GPU request and scheduling-plan contracts.
- [ ] Map contracts onto Inference KServe/LWS lifecycle and propose the concrete request shape and implementation order.
- Scope: source review and recommendation; no cluster mutations, code implementation or commits.

## 2026-09-30 Governance HTTP lifecycle acceptance

Scope: only new cluster 192.168.102.68; no old project changes, no commits or pushes.
- [x] Create isolated temporary tenants via public API; verify unauthenticated and cross-tenant rejection.
- [x] Exercise Model/versions/import and Inference create/get/list/operation via Governance HTTP.
- [x] Exercise stop/start/restart/update through HTTP and real Higress completion; diagnose observed failures.
- [x] Delete test-owned services/models/tenants and exact residual resources; record evidence and remaining limits.

## 2026-10-03 fresh Governance regression

Scope: explicit new cluster only; existing deployed images; HTTP business APIs for all lifecycle changes. No database edits, kubectl runtime patching, commits or source pushes. GPU out of scope.
- [x] Preflight current capacity and pinned request fixture; create isolated tenants/model/version via HTTP.
- [x] Import Qwen2-0.5B from ModelScope and verify ready artifact.
- [x] Create KServe service via Governance; verify real Higress completion and dynamic limiter key.
- [x] Stop/start/restart/resource update via Governance, checking resulting behavior.
- [x] Delete via API and observe automatic materialization cleanup across generations.
- [x] Remove temporary catalog/tenant fixtures, distinguish any operator cleanup, document evidence.

## 2026-10-08 Higress inference API key protection

- [x] Confirm the unauthenticated `/v1/completions` exposure and inspect the
  available Higress Key Auth mechanism.
- [x] Mirror the pinned Higress `key-auth:2.0.0` Wasm image into the new-cluster
  Harbor mirror without changing the old environment.
- [x] Enable a separate `ani-inference-key-auth` plugin with a protected
  `x-api-key` consumer credential; keep the real value outside Git.
- [x] Verify no key, invalid key and query-string key return 401, while the
  valid header key returns a real completion and existing gateway plugins stay
  active.
- [x] Add the non-secret deployment template and key rotation instructions.

## 2026-10-08 Resource Storage preliminary investigation

Scope: user confirms Storage belongs inside ani-resource-service. Prototype and requirements will follow; this turn only investigates existing capabilities and integration boundaries.
- [x] Inspect Resource main architecture, package inventory and remote branch names.
- [ ] Check non-main image/Ceph branches for implemented Storage behavior.
- [ ] Trace current Model/Inference storage consumers.
- [ ] Summarize verified baseline and provisional design inputs.
- Deferred by user: requirements clarification, alternative evaluation, formal design, review, implementation plan and code. No visual question at this stage.

## 2026-10-15 contract-driven real-chain revalidation

- [x] Read the current Model, Inference, Governance and deployment contracts;
  record exact identities, API fields, namespaces, images and gateway rules.
- [x] Preflight only the new `.clusterenv` context and current deployed objects;
  do not touch the old environment.
- [x] Run the complete public chain: Governance model import, model readiness,
  inference creation, operation readiness, Higress completion and API deletion.
- [x] Exercise cleanup and report every blocker separately; do not claim GPU or
  distributed inference without GPU capacity.

### Current run result (2026-10-15)

- Model `origin/master` (`37fe220`) and Inference `origin/main` (`ceacc40`)
  were built as fresh tag-only images and deployed to the new cluster.
- The cluster has only `local-path` and no default StorageClass. This run
  passed `local-path` explicitly to Helm; the default Inference source still
  falls back to `cephfs`, so that source/chart contract remains unmerged.
- Governance `main` was rebuilt and deployed as
  `main-f851d88-20261015`; its Model/Inference BFF HTTPRoute is now Helm-owned,
  enabled by default in the local uncommitted chart, and accepted by Higress.
- The BFF route was verified through Higress: no `x-api-key` returns 401,
  valid key plus valid platform credentials returns 200, and Model/Inference
  requests with a platform token reach Gov and return the expected tenant
  permission 403. A temporary tenant role grant chain was also verified
  internally and cleaned up.
- The complete public import/create/completion chain passed against this latest
  Gov image. GPU and distributed inference remain unverified because the
  cluster has no GPU capacity. StorageClass default absence is intentionally
  not a blocker for this plan.

### Route recheck and latest chain evidence (2026-10-15)

- [x] Verify the new cluster context and remove/confirm absence of temporary
  test Jobs, PVCs, Pods, Model and ModelVersion data.
- [x] Verify the Helm-owned Governance BFF HTTPRoute, including the exact
  `/api/v1/models:import` action path, through Higress.
- [x] Verify the separate `/v1/completions` model-router route responds at the
  gateway; an unknown model is rejected by the router.
- [x] Reach ModelScope import through the public route; the importer retried
  after one failed Job attempt and the Model service recorded the version as
  ready.
- [x] Rerun the poller so a transient `BackoffLimitExceeded` while the task is
  still `importing` is treated as retryable, then execute Inference create,
  real Higress completion and API deletion.

### Final public-chain evidence (2026-10-15)

- Tenant authentication, Model creation, ModelVersion creation and ModelScope
  whole-repository import all passed through the Governance BFF. The 0.5B
  artifact reached `ready` with 999,602,688 bytes and a recorded SHA256.
- Inference creation returned 202 and eventually reached `succeeded/complete`;
  the service became `ready` and `published` with the caller-provided complete
  command preserved exactly. The CPU run used 4 CPU and 8Gi memory.
- Higress `POST /v1/completions` returned HTTP 200 with a non-empty completion.
  API deletion reached `succeeded/complete`, including publication withdrawal,
  runtime deletion and quota release. The temporary tenant, Model,
  ModelVersion, KServe objects, importer Job, PVC and download Secrets were
  then removed.
- The first 8Gi CPU attempt exposed vLLM's default KV-cache sizing and OOMed;
  the successful run supplied `VLLM_CPU_KVCACHE_SPACE=4` in the caller's
  command. No service code appended or synthesized that engine argument.
- This validates the CPU single-instance public path only. GPU, LWS and
  distributed inference remain unverified because the new cluster reports no
  GPU capacity.

### Governance management API coverage audit (2026-10-15)

- [x] Compare Model/Inference downstream proto methods with Governance's
  public proto, generated HTTP registration and BFF adapters.
- [ ] Add the missing Model management operations: external ModelScope/Hugging
  Face search, Model deletion, ModelVersion deletion, upload URL and download
  URL.
- [ ] Add the missing Inference lifecycle and operation-list operations:
  update, start, stop, restart and list operations.
- [ ] Regenerate Governance protobuf/HTTP code, add permissions and verify each
  new public path through Higress without changing the old environment.
