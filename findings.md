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
- New cluster has no PostgreSQL, MinIO, APISIX/Gateway API, KServe, LWS or
  Kubeflow installation. The Kubeflow proposal explicitly says the full
  distribution is not needed for Model import; those components are optional
  execution layers.
- Model process requirements from `cmd/ani-model-service/main.go` are a
  PostgreSQL DSN plus migrations, a direct MinIO/S3 adapter, Kubernetes import
  Job RBAC and an immutable import image. The process has no local principal
  fallback; a trusted IAM/Gateway resolver is required for tenant-scoped RPCs.
- Full Inference lifecycle additionally needs the Inference PostgreSQL schema,
  Model gRPC TLS endpoint, Inference CRD/controller permissions, materializer
  image, APISIX HTTPRoute publication target, and GPU/vLLM capacity for a real
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

## 2026-09-21 new-cluster gateway

- The new cluster initially had no Gateway API CRDs, GatewayClass, Gateway or
  APISIX namespace. Gateway API standard v1.6.1 was applied first, followed by
  APISIX chart 2.17.0 with Ingress Controller 2.2.0.
- The APISIX chart's legacy `ani-block` storage class was not reused. The
  new-cluster values use `rook-ceph-block` and Harbor-mirrored image tags so
  the nodes do not need direct Docker Hub or GHCR access.
- `GatewayClass/ani-apisix` and the `ingress-apisix` Gateway are both
  Accepted/Programmed. The proxy NodePort responds, but no HTTPRoute exists;
  the old route must remain unapplied because it names the legacy E2E
  namespace and vLLM Service.
- APISIX provides the HTTP gateway/inference routing boundary only. It does
  not provide this repository's Model HTTP-to-gRPC business adapter or the
  trusted principal resolver, so Model RPCs remain intentionally gated while
  IAM/TLS are deferred.

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
  operation and returned a real HTTP completion through APISIX at
  `192.168.102.68:30090` using the mirrored vLLM CPU image.
- A separate update test with the initial 4 CPU/8Gi service remains stuck in a
  `resource_work` stale-generation retry loop after repeated status notifications;
  successful create/inference validation uses a new 2 CPU/4Gi service and is
  unaffected. This is the next lifecycle fix, not a blocker to create/invoke.

- 2026-09-23 materializer boundary recheck: a temporary HTTP server served a 537,927,680-byte (513.0 MiB) tar; with MODEL_MAX_BYTES equal to the recorded artifact size, download, SHA256 verification (`6d274cf309f91f8498aa8d3fcef7925a8a456654d77c9eccbc2950f38ddc8cf1`) and extraction all passed. The Job's 512Mi memory limit is unrelated to the artifact-size limit.

## 2026-09-23 larger model validation

- Qwen2.5-1.5B-Instruct pinned revision `989aa7980e4cf806f80c7fef2b1adb7bc71aa306` produced a 3,098,992,640-byte tar with SHA256 `d0f3c1294ac34d9ca84858cf93bfe4dbae8ebdfa2a317d1f90970b1dd85f11dd`. The ModelVersion size persistence path was missing `SizeBytes` in both the service projection and PostgreSQL adapter; the regression test and both assignments are now fixed.
- The 1.5B archive materialized into a 6Gi CephFS PVC, vLLM CPU became ready with 3 CPU/6Gi request and 8Gi limit, and APISIX returned HTTP 200 with non-empty completion output.
- New cluster capacity has no GPU and about 15Gi allocatable memory per worker. A single 1.5B CPU runtime is viable but concurrent 1.5B runtimes pressure PostgreSQL and scheduling; 3B/7B is deferred until larger nodes/GPU are available.
- Kubernetes endpoint names cannot contain dots. The API currently accepts dotted Inference service names and then retries forever after the endpoint Service render fails; safe names use hyphens. Multiple `/v1` HTTPRoutes also need an explicit publication selection policy.
- The invalid dotted-name test Deployment was scaled to zero after validation to release worker capacity. Its active create operation remains pending because the current API has no cancellation RPC and rejects Stop/Delete while an operation is active; the stale operation needs a later lifecycle/admin fix.
