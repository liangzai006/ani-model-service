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
  Job image in the isolated `ani-model` namespace.
- [x] Create the separate `ani_inference` database, apply all twelve
  migrations, and deploy the refactored Inference control plane with its
  Kubernetes runtime composition and namespace-scoped RBAC.
- [x] Install the pinned materializer image reference and CephFS storage class
  configuration; keep actual model materialization pending the deferred Model
  trusted identity/TLS and Quota provider boundaries.
- [x] Install Gateway API v1.6.1 standard CRDs and APISIX 2.17.0 with the
  isolated `ingress-apisix` namespace, new Harbor image tags, and the new
  cluster's `rook-ceph-block` storage class.
- [x] Apply the new-cluster `GatewayClass` and `Gateway`; leave HTTPRoute
  creation pending while identity, Model TLS, and the serving runtime remain
  deferred. Do not apply the legacy E2E route.

## New-cluster runtime wiring (2026-09-22)

- [x] Remove the Inference-only `ANI_KUBERNETES_ENABLED` switch. The service
  now requires and starts its Kubernetes runtime in every deployment.
- [x] Install the Inference CRD and LWS controller/CRDs on the new cluster.
- [x] Deploy the new-cluster ServiceAccount, namespace Role and RoleBinding;
  verify the account can manage only the `ani-inference` runtime resources.
- [x] Verify the new image starts, opens the gRPC/admin listeners and reaches
  the Kubernetes manager without cache-sync errors.
- [x] Superseded by the user's direct-access requirement: use plaintext Model
  gRPC and request tenant metadata, plus fixed quota capacity 10. No external
  IAM, trusted identity or TLS prerequisite remains in the current validation.

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
- [x] Materialize the archive to CephFS, start vLLM CPU with bounded context, and return a real completion through APISIX.
- [x] Verify the 1.5B path without modifying old-cluster resources; recheck the existing 135M route after the temporary route-isolation check.
- [x] Record current-node capacity and the 3B/7B deferral.
- [ ] Fix endpoint naming for dotted service names and define deterministic multi-model `/v1` publication selection.
