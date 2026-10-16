# Current evidence (2026-09-16)

## 2026-09-18

- Model deletion protection is verified against real PostgreSQL: model and single-version deletion serialize with ready reads/version creation, active references reject deletion, dependency failure rolls back, and retries are idempotent.
- Inference version-reference gRPC is verified against real PostgreSQL with running/stopped/failed/deleting and old-generation specs treated as active; deleted services and other tenants are excluded.
- The actual Inference checkout contains the reference integration test and completed-Job Model revalidation regression test. Full `go test`, `go vet`, and `go build` pass there; Model `make verify` also passes.

- Actual Inference patch application succeeded; focused model and cmd package
  tests passed on host after write-back.
- Cluster context: kubernetes-admin@kubernetes. dev-phys-02: 2 allocatable RTX4090,
  kubercloud: 2 allocatable RTX4090; active Pod GPU requests counted as zero.
  dev-phys-03 reports zero allocatable GPU. Nodes have abundant CPU/RAM.
- StorageClass cephfs and nfs use CephFS, WaitForFirstConsumer, Retain. ani-block
  and ani-rbd-ssd use RBD. Use a new dedicated PVC; retain successful evidence.
- Cached GPU vLLM image on all nodes:
  docker.changqingyun.cn/ani/vllm-openai@sha256:6cf9808ca8810fc6c3fd0451c2e7784fb224590d81f7db338e7eaf3c02a33d33
- Existing CRD: inferenceservices.ani.kubercloud.com; LWS CRD/controller present.
- Model previous imported fixture is tiny-gpt2 config.json only, inadequate for
  inference. Official SmolLM2-135M-Instruct repo lists 269 MB model.safetensors,
  config and tokenizer; architecture LlamaForCausalLM, head size 64.
- Inference Runner requires quota → CR → materialize → runtime → observation →
  publication → invocation. ModelPort cannot be the metadata client. RuntimeSpec
  already holds model UUID/ref/digest from Inference-owned tables.
- Default composition still lacks Model/Quota/Publication/Invocation provider
  wiring and inbound identity. Existing renderer has no model volume or probes.
- Model supports signed upload then checksum-verified CreateModelVersion.
  Reusing this avoids extending the single-file remote importer for this goal.
- Local recycling PostgreSQL currently contains Model tables, no inference_*
  tables; existing ani_inference DB should be checked independently.

Sources: https://huggingface.co/HuggingFaceTB/SmolLM2-135M-Instruct/tree/main
and its config.json; https://docs.vllm.ai/en/latest/models/supported_models/.

## 2026-09-21 new-cluster preflight

- `.clusterenv` lists `192.168.102.68` (`ani-k8s-cp`) and workers
  `192.168.102.72`/`.73`; it contains no PostgreSQL, MinIO, registry, or
  kubeconfig settings. The existing local kubeconfig still points to the old
  `kubernetes-admin@kubernetes` cluster in the `10.10.1.x` network.
- New cluster: Kubernetes v1.37.0, all three nodes Ready. Only base/Calico and
  `rook-ceph` namespaces exist. No Model/Inference namespace or application
  workload exists.
- New storage: default `cephfs` plus `rook-ceph-block`, both freshly created
  with `WaitForFirstConsumer`; CephFilesystem and OSD/CSI pods are Ready.
  Ceph has 270 GiB raw capacity and reports only insecure AES client-key
  warnings. No CephObjectStore or S3 endpoint exists.
- New nodes have no `nvidia.com/gpu` allocatable resource and no `nvidia-smi`.
  GPU operator/device plugin and a real vLLM completion are therefore not
  available in the new environment.
- New cluster has no PostgreSQL, MinIO, an HTTP gateway/Gateway API, KServe, LWS or
  Kubeflow installation. The Kubeflow proposal explicitly says the full
  distribution is not needed for Model import; those components are optional
  execution layers.
- Model process requirements from `cmd/ani-model-service/main.go` are a
  PostgreSQL DSN plus migrations, a direct MinIO/S3 adapter, Kubernetes import
  Job RBAC and an immutable import image. The process has no local principal
  fallback; a trusted IAM/Gateway resolver is required for tenant-scoped RPCs.
- Full Inference lifecycle additionally needs the Inference PostgreSQL schema,
  Model gRPC TLS endpoint, Inference CRD/controller permissions, materializer
  image, a Gateway API HTTPRoute publication target, and GPU/vLLM capacity for a real
  completion.
- Isolation rule: all future mutations must use a dedicated new-cluster
  kubeconfig/SSH wrapper and new namespaces/secrets; never edit `~/.kube/config`
  or use the old context for apply/delete/helm operations.

## 2026-09-21 new-cluster foundation and Model schema

- Created only the new-cluster `ani-foundation` namespace. PostgreSQL and
  MinIO are single-instance StatefulSets backed by new-cluster
  `rook-ceph-block` PVCs; both Services are internal ClusterIP Services.
- PostgreSQL is ready in the new `ani_model` database, and the Model migration
  Job completed all six repository migrations. The resulting public schema has
  `models`, `model_versions`, `model_artifacts`, `model_import_tasks`, and
  `audit_events`; `models.model_id` is present.
- The first migration Job completed without applying files because projected
  ConfigMap entries are symlinks and the manifest used `find -type f`. The Job
  was corrected to include symlinks, the empty database was independently
  verified, and the Job was rerun once successfully. No duplicate migration
  was run against an already populated schema.
- MinIO is ready and the authenticated `ani-models` bucket exists. No image
  registry, TLS/IAM, gateway, GPU, Kubeflow, E2E namespace, or Inference
  database was installed.

## 2026-09-21 new-cluster Model deployment

- Existing Harbor is reachable anonymously from the new control plane and
  both workers. It already contains the repository's current import image, so
  no registry installation was needed.
- The Harbor tag previously named `model-service:remote-import-20260921-r3`
  belongs to a different module (`github.com/kubercloud/ani/services/model-service`)
  and must not be used for this repository. The new deployment uses the
  repository-built digest recorded in the Model deployment execution record.
- The scratch Model image must be built with `CGO_ENABLED=0` and must include
  readable `configs/config.yaml`; otherwise it either fails at exec time or
  exits with `load config: ... permission denied`.
- Model service readiness is healthy with PostgreSQL, MinIO, and the import
  worker configured. Business RPCs still require a trusted principal because
  the composition root intentionally has no local identity fallback.
- Current shared-bucket mode is deliberate: `cmd/minio-provision` writes a
  bare `imports/<task>/model.tar` key in tenant-bucket mode while the storage
  adapter looks up a tenant-prefixed key. Do not enable tenant buckets until
  the mismatch has a regression test and a corrected contract.

## 2026-09-22 new-cluster Inference control plane

- The new PostgreSQL database is `ani_inference`, owned by the existing
  `ani_model` role. The migration Job applied the repository's twelve SQL
  files and the database contains 11 public tables whose names begin with
  `inference_`.
- The initial scratch image failed because the checked-in config file was
  mode `0600`, which prevented the image's non-root UID 65532 from reading it.
  The image was rebuilt with the same source config at mode `0644`; the new
  immutable r3 digest is the deployed image.
- The Inference composition root previously held readiness false when no
  Kubernetes background servers were configured. The small
  `readyOnStartForBackground` change makes the control-plane-only deployment
  ready while preserving background readiness gating when runtime servers are
  present; focused tests pass.
- The Deployment intentionally does not enable Kubernetes reconciliation and
  exposes only ClusterIP Services. There is no new-cluster HTTPRoute, public
  inference endpoint, IAM resolver, Model TLS adapter, quota/publication
  provider, materializer image, or GPU runtime yet.

## 2026-09-18 C acceptance

- The real `TestImportManifestRepositoryE2E` passed with a random tenant and
  HuggingFace `sshleifer/tiny-gpt2@main`. It listed 9 files, streamed a
  4,742,656-byte tar to MinIO, verified the downloaded SHA256
  `361d696078e595f9ef50fdcf9036b08f216f180a7c4f6def9a63eff3fc114dd4`,
  persisted the artifact, and transitioned the version to ready.
- `GetImportTask` returned completed with 100% progress and completion time.
  After injecting a failed state, `RetryImportTask` returned pending with
  attempt count and progress reset to zero; the restarted worker completed the
  same task again. No credentials were written to the repository.

## 2026-09-18 D version switch

- Inference Update now accepts `model_version_id` and resolves it through the
  versioned Model gRPC before persisting the next generation. A target that is
  pending, missing, or has conflicting artifact/engine facts is rejected; an
  omitted target preserves the existing snapshot.
- The actual Inference checkout passed full tests, vet, build, Buf lint, and a
  real PostgreSQL generation test asserting the new version UUID in generation
  2. Generic runtime limits and Kubernetes lifecycle/recovery remain open.

## 2026-09-19 lifecycle preflight

- Read-only cluster preflight passed: context `kubernetes-admin@kubernetes`, Kubernetes v1.36.1, dedicated namespace `ani-inference-e2e-20260914` exists and is clean, `cephfs` is RWX-capable, LWS v0.10.0 is Ready, and GPU capacity is available on `dev-phys-02` (the two prior live deployments consume the two GPUs on `kubercloud`). Existing successful workloads were not touched.
- A new formal Inference process cannot pass readiness in the current checkout: `buildKubernetesServers` wires only PostgreSQL admission and Kubernetes runtime; `Runner.Model`, `Runner.Quota`, and `Runner.Publication` remain nil. `ReadyCheck` therefore stays false and `/readyz` is expected to remain 503.
- External identity/provider boundaries are also unresolved: Inference has no IAM middleware that injects trusted tenant/actor context; Model currently exposes plaintext gRPC while Inference requires TLS 1.3; Model calls require a valid trusted Principal; no versioned Quota or Publication provider implementation/endpoint is present.
- The Inference CRD CEL rules were synchronized with the implemented runtime rules: positive Deployment replicas and `leader_worker_set.workerReplicas >= 1`.

## 2026-09-22 new-cluster real validation

- Read-only inspection confirms the new cluster has Model and Inference
  control-plane Pods, Inference CRD/LWS/Gateway, but no InferenceService,
  runtime Deployment, PVC, materialization Job or HTTPRoute yet.
- New-cluster nodes have no GPU and only Python/model-service images cached;
  a real CPU runtime image is therefore required for the final request.
- The Model API supports direct request tenant scoping and its Deployment has
  direct MinIO plus Kubernetes import Job configuration.
- Inference protobuf requests do not contain a tenant field. The service
  currently requires tenant context, while the production identity middleware
  is not installed in the new validation Deployment. This is the first active
  blocker to creating an Inference resource through gRPC; do not bypass it by
  writing the database directly.

## 2026-09-23 real new-cluster completion

- Model r8 is static, contains CA roots, uses HF revision manifests with
  `blobs=true`, and has a 30s gRPC timeout. Focused Model tests passed.
- Direct Hugging Face import from new workers is blocked by cluster egress;
  the formal signed upload API path was used instead. ModelVersion
  `7e4b355c-9cbd-4f5e-ba98-c75e3aadedc1` is ready after MinIO object existence
  and checksum verification.
- Inference r11 embeds a safe tar extractor that accepts `./` root directory
  entries and carries previous generation bindings while a replacement is
  being fenced. Focused Postgres/model/Kubernetes/service tests passed.
- New-cluster service `ae68574f-3127-48f3-9a30-6788684797c8` completed its
  operation and returned a real HTTP completion through the then-current gateway (now retired) at
  `192.168.102.68:30090` using the mirrored vLLM CPU image.
- A separate update test with the initial 4 CPU/8Gi service remains stuck in a
  `resource_work` stale-generation retry loop after repeated status notifications;
  successful create/inference validation uses a new 2 CPU/4Gi service and is
  unaffected. This is the next lifecycle fix, not a blocker to create/invoke.

- 2026-09-23 materializer boundary recheck: a temporary HTTP server served a 537,927,680-byte (513.0 MiB) tar; with MODEL_MAX_BYTES equal to the recorded artifact size, download, SHA256 verification (`6d274cf309f91f8498aa8d3fcef7925a8a456654d77c9eccbc2950f38ddc8cf1`) and extraction all passed. The Job's 512Mi memory limit is unrelated to the artifact-size limit.

## 2026-09-23 larger model validation

- Qwen2.5-1.5B-Instruct pinned revision `989aa7980e4cf806f80c7fef2b1adb7bc71aa306` produced a 3,098,992,640-byte tar with SHA256 `d0f3c1294ac34d9ca84858cf93bfe4dbae8ebdfa2a317d1f90970b1dd85f11dd`. The ModelVersion size persistence path was missing `SizeBytes` in both the service projection and PostgreSQL adapter; the regression test and both assignments are now fixed.
- The 1.5B archive materialized into a 6Gi CephFS PVC, vLLM CPU became ready with 3 CPU/6Gi request and 8Gi limit, and the then-current gateway (now retired) returned HTTP 200 with non-empty completion output.
- New cluster capacity has no GPU and about 15Gi allocatable memory per worker. A single 1.5B CPU runtime is viable but concurrent 1.5B runtimes pressure PostgreSQL and scheduling; 3B/7B is deferred until larger nodes/GPU are available.
- Kubernetes endpoint names cannot contain dots. The API currently accepts dotted Inference service names and then retries forever after the endpoint Service render fails; safe names use hyphens. Multiple `/v1` HTTPRoutes also need an explicit publication selection policy.
- The invalid dotted-name test Deployment was scaled to zero after validation to release worker capacity. Its active create operation remains pending because the current API has no cancellation RPC and rejects Stop/Delete while an operation is active; the stale operation needs a later lifecycle/admin fix.

## 2026-09-24 KServe LLMI/LWS validation

- The new cluster already had LWS v0.10. KServe 0.16 LLMI CRDs and the
  `llmisvc-controller-manager` were installed independently in `kubeflow`,
  using the Harbor-pinned controller image; the KServe 0.15 controller and its
  ConfigMap were left unchanged.
- The LLMI webhook initially timed out because the controller pod was covered
  by Istio injection and its namespace NetworkPolicy did not allow the API
  Server to reach port 9443. Disabling injection for this controller and
  adding a port-only webhook NetworkPolicy fixed creation of the empty pipeline
  config.
- `LLMInferenceService/lws-smollm2-cpu` with `parallelism.pipeline: 2` created
  `lws-smollm2-cpu-kserve-mn` with two ready pods on
  `ani-k8s-worker-2` (192.168.102.73) and `ani-k8s-cp` (192.168.102.68).
  KServe reported `MainWorkloadReady=True`, and the generated workload Service
  exposed both pod endpoints.
- The first real vLLM attempt OOM-killed at a 2Gi limit and was not treated as
  a KServe failure. The canary was switched to a caller-provided lightweight
  HTTP command to prove LWS lifecycle and cross-node Service routing. This is
  not a model-inference result; GPU/distributed vLLM validation remains open.
- The LLMI readiness observer now accepts KServe 0.16 status without an
  `observedGeneration` field, rejects an explicitly stale LLMI status, and
  reports LWS worker/group counts only from the current rollout. Full Inference
  tests, vet and build pass after this fix.

## 2026-09-24 real Inference API to KServe LWS validation

- Applied migration `000015_inference_kserve_llm_binding.sql` only to the new
  cluster's `ani_inference` database; it extends the runtime binding kind
  check for `KServeLLMInferenceService`.
- Deployed the static Inference image
  `docker.changqingyun.cn/ani/inference-service:ani-inference-new-20260924-lws-r3@sha256:1e46f1114e9c3cc403b110ec969563844daa959e7310f2d4a19fd629297e7924`.
- A real gRPC `CreateInferenceService` request with
  `runtime.provider=kserve`, `mode=RUNTIME_MODE_LEADER_WORKER_SET`,
  `workerReplicas=1`, and a caller-supplied command created service
  `de14fc07-3702-4b75-a3a1-ec7cfb1de153`; its operation reached `succeeded`.
- KServe created an LLMI, native `LeaderWorkerSet/lws-api-smoke-kserve-mn`,
  and two ready Pods on `ani-k8s-worker-2` (192.168.102.73) and `ani-k8s-cp`
  (192.168.102.68). Its generated Service had both endpoints and its
  HTTPRoute was `Accepted` and `ResolvedRefs`.
- The observer initially rejected KServe's automatic `/mnt/models` mount, LWS
  environment variables and Kubernetes defaults. `kserveContainerMatches` now
  compares caller-owned command/image/resource/port fields exactly and treats
  expected env/mount entries as required subsets. A regression test covers it.
- A second real API create (`lws-api-gateway`, service
  `6978cea5-a5f7-4799-b329-a81e7c56f2fe`) reached `succeeded`; a POST to
  `http://192.168.102.68:30090/v1/completions` with the route model header
  returned HTTP 200 through Higress and the LWS probe response. Its Delete API
  operation also reached `succeeded`.
- Both test services, their KServe/LWS objects, routes, PVCs, Jobs, Secrets
  and completed download Pods were removed by exact service-id ownership
  labels. The existing `smollm2-135m-cpu-real` service remains ready and was
  not modified. The cluster still has no GPU, so this proves LWS/KServe
  orchestration and gateway routing, not distributed GPU model inference.

## Governance / Accelerator integration review (2026-09-25)

- Source snapshots are being read under `/tmp/ani-integration-review-ATMCYT/`.
- The current Inference composition uses one Kubernetes manager/client from ANI_KUBECONFIG or in-cluster credentials; a future GPU cluster_id cannot be interpreted as a node selector or silently dispatch to another cluster.
- The existing HTTPRoutePublisher is the Higress inference data-plane publication adapter; distinguish it from management API routing.

## Upstream Governance / Accelerator review (2026-09-25)

- `ani-governance` HEAD `d27847b99479553b1a56fc84bc4ee8f1a0992e1e` is an admin/tenant BFF and governance/quota control plane, not the inference data-plane gateway. Its active registrations are Config, Network and Accelerator/Quota BFFs; the Model client/proto/route was removed on 2026-09-21. It has no KServe, Higress, HTTPRoute or `/v1/completions` ownership.
- `ani-governance` has a documented GPU owner seam but the formal production build currently leaves the owner registry empty and GPU execution/catalog readiness disabled. The real `ani-inference` owner adapter, release identity map and service-to-service contract are still absent; test owner code is not production integration.
- `ani-accelerator-service` fixed source SHA is `1d32dd9a9173b8869fa0ef2ae64e20b88f2ca0a3`. Its consumer contract uses `GpuRequest{cluster_id,pool_id,profile_id,profile_version,replicas,devices_per_replica,container_name}`; it does not define `pool` without `_id`.
- Accelerator `ResolveGpuRequest` returns a frozen `ResolvedGpuPlan`; it does not reserve or lock quota/capacity. The plan includes `runtime{scheduler_name,queue_name,node_labels,pod_annotations,limits_per_container,runtime_class_name,recipe_version}` and `encoding/totals`.
- The contract says Inference owns workload rendering, model readiness, deletion and release facts; Governance owns quota/usage synchronization; Accelerator owns supply/profile/inventory/capacity/binding. A real Inference adapter must embed `GpuOwnerCreateAttachment`/`GpuOwnerDeleteAttachment`, sign actual GPU Pod assertions, observe release, and report quota release after complete deletion.
- Current `ani-inference-service` has one Kubernetes client/config selected by `ANI_KUBECONFIG` or in-cluster config. It does not route a request to another Kubernetes cluster from `cluster_id`; multi-cluster requires a separate config/client registry and is not safe to infer from node labels.
- KServe renderer currently owns caller container image/command/resources and does not consume Accelerator RuntimeFragment scheduler/queue/node labels/pod annotations/GPU limit keys. That is the main implementation seam.

## 2026-09-30 corrected verification scope

Prior fresh 1.5B and LWS runs used /tmp/ani-fresh-api.py against Governance NodePort 30095; the last response incorrectly reduced that evidence to gRPC-only. Historical status documents are stale. This run verifies lifecycle and isolation using actual HTTP responses, never database status edits.

2026-09-30 results: all six KServe lifecycle operations succeeded through Governance HTTP. Generation 5 changed CPU 4→3; Higress returned real generation after create/start/update. GET after deletion returns tombstone 200, list excludes it. Four generation-specific materialization sets persisted after delete and needed exact-owner cleanup. Signed URLs use internal MinIO hostname; storage-only round-trip used port-forward. Prior status statements about missing HTTP path/current KServe stale-generation were corrected.

## 2026-10-08 Resource Storage preparation

- User explicitly assigns Storage to ani-resource-service; prototype and requirements are forthcoming.
- Resource main a4ca2a0fcb18346fff27f682a5244874ca4c60c8 has Network implementation; ADR-0006 and modularization plan reserve api/storage/v1 and internal/{biz,data,service}/storage for a future actual slice in the same process.
- Remote heads additionally include codex/image-mvp-20260930 (3d15d09a9c7edfd5fccc6a95412c4877b82bff15) and codex/install-ceph (f198a1ea75549d24a95aa52e609e458a0712b234); their content is being checked, so main inventory alone is insufficient to characterize every branch.
- Existing Network migration validates exclusive ownership of its public tables; adding Storage requires explicit database/schema and migration design. Do not casually append Storage tables to that migration set.

- The `codex/image-mvp-20260930` branch (3d15d09) contains an Image domain and Harbor-oriented specs/code. Its own scope explicitly excludes model weights, VM disks and Helm packages; it is OCI image registry management, not MinIO/Ceph model-object storage. The Image MVP spec is marked “待实施”, so branch code/specs are not proof of main deployment or product acceptance.
- The `codex/install-ceph` branch (f198a1e) contains Ceph deployment/runbook assets, not a Resource Storage domain/API implementation.
- Current Model storage is an external-object contract: signed upload/download URLs, object existence and SHA256 verification. `ani-model-service` can dial a Storage gRPC address or direct MinIO, but Kubernetes import still requires direct MinIO configuration; this must be resolved before making Resource Storage the sole backend.
- Current Inference owns generation-scoped RWX model PVC, download Job/Secret, ownership labels, readiness verification and cleanup. A Resource Storage volume API must not silently take over these objects; the prototype must choose whether Storage only supplies object/volume primitives or also owns materialization.

## 2026-10-15 contract audit before real-chain run

- Model README and `docs/model-inference-fields.md` require the sequence SearchExternalModels (optional) -> CreateModel -> CreateModelVersion -> ImportModel. Search does not persist or download; import is the whole provider repository. ModelVersion carries `engine_type`, `engine_image`, and one complete `startup_command` string; no service may split, append, or infer engine arguments.
- Model Helm requires `ani-system`, external cert-manager/`ClusterIssuer/ani-internal`, pre-existing PostgreSQL/S3/Secrets, and default StorageClass when `import.storageClass` is empty. Its service certificate is `ani-model.ani.internal`; Governance client certificate must have exactly one DNS SAN `ani-governance`.
- Inference Helm requires the same `ani-system`, external cert-manager/issuer, Model gRPC Service, default StorageClass for model PVCs, KServe 0.16/LLMI, LWS, Gateway API, Higress and API-key protection. Governance ordinary gRPC must use TLS 1.3/mTLS, client DNS SAN exactly `ani-governance`, and `x-ani-tenant-id` plus `x-ani-actor` metadata with the registered format.
- Governance current main owns public HTTP BFF APIs and forwards to Model/Inference over mTLS. Gov's canonical integration register requires exact caller-provided command and no engine argument synthesis; `model_version_id` is required for inference creation, while engine/image/command are still supplied by the final create request.
- Governance Helm defaults downstream Model/Inference clients and certificate issuance off; a real deployment must explicitly enable them and provide addresses/server names/CA. This is a likely deployment preflight item, not a code assumption.
- Current `.clusterenv` names the new hosts 192.168.102.68/72/73. The local kubectl context is `kubernetes-admin@kubekey`; its API server must be checked before any write, and all commands must pass the verified current context by relying on the current kubeconfig as documented.

## 2026-10-15 branch correction

- The remote `feat/gov-model-inference-flow-20261012` branches in Model and Inference are strict ancestors of `origin/master`/`origin/main`; they are not newer than the default branches. Model default now includes the feature branch plus PR #2; Inference default includes the feature branch plus PR #4.
- Local Model and Inference worktrees are now on `master`/`main` respectively and fast-forwarded to those remote heads. Existing uncommitted local changes were preserved and not pushed.
- Governance is out of scope for this validation as requested.

## 2026-10-15 default-branch deployment and real-chain result

- Clean worktrees at Model `origin/master` `37fe220768328173a70a0b1f47dbf5139e7fab22`
  and Inference `origin/main` `ceacc40637424d75c4ae0291873f7e8da475fc7b` passed
  their full `go test ./... -count=1` suites. Fresh images were pushed with
  tag-only references: Model `master-37fe220-20261015`, importer with the same
  tag, and Inference `main-ceacc40-20261015`.
- Helm upgrades on the new context `kubernetes-admin@kubekey`
  (`https://192.168.102.68:6443`) completed. Model and Inference pods are Ready
  with those images. The Inference migration first rendered an old reused map
  value as `InvalidImageName`; the failed hook was deleted and a second Helm
  upgrade explicitly supplied the migration image string. No application
  resource was created by that failed hook.
- The new cluster has only `local-path`, with no
  `storageclass.kubernetes.io/is-default-class` annotation. This run passed
  `model.storageClass=local-path` and `import.storageClass=local-path`
  explicitly. The checked-in Inference `origin/main` still maps an empty
  `ANI_MODEL_STORAGE_CLASS` to `cephfs`, which does not exist in this cluster;
  the blank-means-default fix is present only in uncommitted local changes and
  is not part of the default branch image.
- A public API run logged in to Gov, created tenant `real-main-20261015-111144`,
  and logged in as its administrator. Model creation then returned HTTP 403
  `FORBIDDEN`. Read-only platform inspection showed the deployed Gov image
  exposes tenant management under `/admin/v1/*` (not the checked-in proto's
  `/api/v1/admin/*`), and plan 1 has MODEL/INFERENCE module rows but no matching
  role permission grant for the new tenant. Per scope, Gov was not modified.
- The temporary tenant was cleaned and deleted via the platform API. No Model,
  import task, PVC/Job, InferenceService, LWS, HTTPRoute or Higress completion
  was created in this attempt. GPU remains unverified because the cluster has
  no GPU devices.

## 2026-10-15 Governance BFF route correction

- The previous 403 result was from the deployed Gov permission state and did
  not prove a missing network route. Governance `main` now runs from
  `f851d88`, and the BFF HTTPRoute is attached to `higress-system/ani-higress`
  with resolved backend `ani-governance:7788`.
- The route is Helm-owned and covers management paths under `/api/v1/auth`,
  `/api/v1/admin`, `/api/v1/models`, `/api/v1/model-versions` and
  `/api/v1/inference`. The inference data plane remains a separate
  `/v1/completions` model-router route.
- Platform credentials intentionally do not authorize tenant Model/Inference
  operations; those calls require a tenant token with the Model/Inference
  role permissions. A temporary role grant and tenant API read test passed and
  was fully cleaned up.
- The local Governance Chart change is intentionally uncommitted and was not
  pushed. No old cluster was touched.
- The public management entry points are present in the new cluster: Gov owns
  `/api/v1/auth`, `/api/v1/admin`, `/api/v1/models`,
  `/api/v1/model-versions` and `/api/v1/inference`; Inference owns the
  separate `/v1/completions` data-plane route. Model and Inference gRPC
  Services remain ClusterIP and are called by Gov over mTLS, so they are not
  separately exposed at the gateway.
- The exact action path `/api/v1/models:import` must be listed explicitly in
  the Gov HTTPRoute. A prefix match for `/api/v1/models` is insufficient.

## 2026-10-15 public route and lifecycle verification

- The public management chain is reachable through the Helm-owned Governance
  BFF HTTPRoute: `/api/v1/auth`, `/api/v1/admin`, `/api/v1/models`,
  `/api/v1/model-versions`, exact `/api/v1/models:import`, and
  `/api/v1/inference`. Model and Inference gRPC remain internal ClusterIP
  services and are reached by Governance over mTLS.
- The separate Higress data-plane route `/v1/completions` served a real
  completion after the ModelScope import and Inference publication completed.
  API deletion completed publication withdrawal, runtime deletion and quota
  release before test cleanup.
- The ModelVersion's complete startup command was preserved byte-for-byte in
  the created Inference record. The successful CPU run passed the vLLM KV-cache
  environment setting in that request command; application code did not add
  startup arguments.
- A transient importer `BackoffLimitExceeded` while the import was still
  retryable did not prevent the Model worker retry from producing a ready
  version. The run therefore polls the task state rather than treating one
  failed Job attempt as final failure.
- Current new-cluster verification is CPU-only. The cluster has no GPU
  devices, so GPU scheduling and multi-node/LWS behavior remain unverified.

## 2026-10-15 Governance API registration audit

- The Helm HTTPRoute is not the complete API contract. It only forwards the
  `/api/v1/*` paths; the generated Governance BFF registration determines
  which operations actually exist.
- Governance `main` currently registers the core Model flow:
  `CreateModel`, `GetModel`, `ListModels`, `CreateModelVersion`,
  `GetModelVersion`, `ListModelVersions`, `ImportModel`, `GetImportTask` and
  `RetryImportTask`. The corresponding public paths are present and a
  platform-token request reaches the Gov authorization layer with HTTP 403,
  rather than a route or backend 404.
- The Model Service contract also contains `SearchExternalModels`,
  `DeleteModel`, `DeleteModelVersion`, `GetUploadURL` and
  `GetModelDownloadURL`. None of these are present in Gov's
  `admin/service/v1/i_model.proto`, generated HTTP registration, or
  `ModelOwnerClient`; they are not currently available through the management
  entry.
- The downstream Inference contract contains update, start, stop, restart and
  operation-list methods. Gov currently registers only create, delete, get,
  list-services and get-operation. Those lifecycle/list-operation APIs are
  also absent from the management entry.
- Therefore the public CPU create/import/infer/delete chain is valid, but the
  complete Model and Inference API surface is not yet wired through Gov. The
  next implementation slice must extend the Gov proto, generated code and
  BFF adapters before claiming full management API coverage.
