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
