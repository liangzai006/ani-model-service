# Inference Accelerator and KServe Integration Design

## Goal

Extend the new Inference API so a caller can request a managed GPU profile,
resolve it through `ani-accelerator-service`, persist the immutable resolved
plan, and render that plan into KServe Deployment or native LWS workloads.

The first phase is limited to the Inference-to-Accelerator-to-KServe path. It
does not enable Governance quota accounting, IAM, TLS changes for the public
API, multi-cluster execution, or any change to the old environment.

## Current boundaries

- Higress is the inference data-plane gateway for `/v1/completions` and
  HTTPRoute publication.
- `ani-governance` is the sibling business/control-plane gateway for model and
  inference management APIs. It calls Model and Inference gRPC directly; it is
  not behind Higress, does not proxy Higress, and does not own KServe routes.
  The current Governance revision has its Model BFF temporarily removed, so
  this integration remains a separate deployment slice.
- `ani-accelerator-service` owns GPU profiles, supply, capacity and immutable
  plan resolution. `ResolveGpuRequest` does not reserve capacity.
- Inference owns workload creation, KServe/LWS rendering, model readiness and
  deletion observation.
- The old cluster and its resources remain outside this change.

## API shape

Keep CPU and memory in the existing Kubernetes-style maps. Move GPU selection
out of `requests`/`limits` into a nested object. The nested object follows the
Accelerator contract; the upstream field is `pool_id`, not `pool`.

```json
{
  "requests": {"cpu": "4", "memory": "16Gi"},
  "limits": {"cpu": "4", "memory": "16Gi"},
  "gpu": {
    "cluster_id": "cluster-uuid",
    "pool_id": "pool-uuid",
    "profile_id": "profile-uuid",
    "profile_version": 1,
    "replicas": 2,
    "devices_per_replica": 1,
    "container_name": "main"
  }
}
```

The API must reject legacy GPU resource-map entries. GPU selection is enabled
only when the nested `resource.gpu` object is present; when it is omitted, the
request does not use GPU and no accelerator resolution or GPU scheduling fields
are applied. The service must not invent a profile, GPU count, node selector or
engine argument. If a product-facing compatibility field named `pool` is
needed later, it must be an explicit boundary alias to `pool_id`, not a second
persisted contract.

## Resolution and persistence

Create and GPU-affecting Update requests persist the caller-owned GPU request
with the generation. The durable operation then calls
`AcceleratorCatalogService.ResolveGpuRequest` before materialization or runtime
apply. The returned `ResolvedGpuPlan` and its digest are stored with that
generation and reused for retries. A later retry never re-resolves against a
new profile, factor or capacity snapshot.

The resolved request's replica count must match the actual GPU-bearing Pod
count. For LWS, this is the complete Leader/Worker topology, not merely the
number of groups. The implementation must reject an ambiguous topology rather
than silently deriving a different GPU count.

The current public Model/Inference direct tenant path remains unchanged. The
Accelerator client is a separate service-to-service dependency; its upstream
production contract requires TLS 1.3, mTLS and exact service identities. This
phase must not add a plaintext fallback.

## KServe rendering

The KServe adapter consumes only the persisted resolved plan and the existing
caller-owned engine fields. 按 2026-10-14 的当前契约，`engine.command` 是完整命令
字符串，必须逐字保留；业务接口不再接收独立的 `engine.args`。Kubernetes 适配器
只使用固定的 `/bin/sh -c` 包装，不生成任何引擎参数。

The plan runtime fragment is applied to every GPU-bearing KServe template:

- `scheduler_name` becomes the Pod scheduler name.
- `node_labels` become managed node-selection constraints.
- `pod_annotations` and `limits_per_container` are applied exactly after
  validating that the target container matches `container_name`.
- `runtime_class_name` is applied when present.
- `queue_name` and `recipe_version` remain managed plan fields and are not
  caller-overridable.

Deployment and native LWS rendering use the same frozen plan. The LWS leader
and workers must receive the same GPU plan, and the adapter must not add extra
GPU containers, replicas or rolling workers.

`cluster_id` is initially checked against the configured new-cluster identity.
The service must reject a different cluster ID rather than use the current
Kubernetes client against an unintended cluster. A real multi-cluster feature
requires a separate, explicitly registered Kubernetes client per cluster and
is outside this phase.

## Lifecycle and failure handling

- Resolve failures leave the operation retryable and do not create a KServe
  workload.
- A resolved plan is immutable for its generation and is included in durable
  observations.
- Delete first closes and removes the complete KServe/LWS workload, then
  records the GPU Pod range needed for future Accelerator release observation.
- Governance owner attachments, quota release reporting and Accelerator
  `ObserveRelease` are recorded as a later phase because the formal Governance
  owner adapter is not currently registered.

## Validation

- Proto/API tests cover nested GPU parsing, required Accelerator fields,
  rejection of legacy GPU map entries and generation persistence.
- Accelerator adapter tests use a fake gRPC client to verify request mapping,
  plan digest persistence, retry reuse and cluster-ID rejection.
- KServe renderer tests verify scheduler, node labels, annotations, runtime
  class and per-container limits for both Deployment and LWS, while asserting
  command/args are unchanged.
- Existing CPU KServe/LWS and Higress tests remain unchanged and continue to
  pass. No test applies resources to the old cluster.

## Non-goals

- No Governance Model/Inference BFF or quota-owner implementation in this
  accelerator slice.
- No public IAM, trusted identity or TLS contract change.
- No automatic profile selection or GPU count inference.
- No engine startup command construction.
- No Notebook, Dashboard, Trainer or unrelated Kubeflow work.
- No old-environment migration or cleanup.
