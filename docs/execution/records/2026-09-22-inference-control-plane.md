# New-cluster Inference control plane (2026-09-22)

Scope: only the Kubernetes cluster described by `.clusterenv`, using the
control plane at `192.168.102.68` and its `/etc/kubernetes/admin.conf`. The
existing local kubeconfig context and old project resources were left
untouched.

## Database and migrations

- Created the separate `ani_inference` database in the new cluster's existing
  PostgreSQL StatefulSet, owned by the existing `ani_model` role.
- Created a namespace-local `ani-inference/inference-database` Secret with a
  DSN for the internal `postgres.ani-foundation` Service. Secret values were
  not written to this record.
- Applied the twelve migrations from the Inference repository with Job
  `ani-foundation/inference-migrations-20260922`. The Job completed, and an
  independent query returned `inference_tables=11`.

The migration manifest is
`docs/deploy/foundation/inference-migration-job.yaml`. It uses the pinned
PostgreSQL client image and accepts the symlinked files produced by a
ConfigMap volume.

## Inference deployment

- Namespace: `ani-inference`.
- Deployment: `ani-inference-service`, one replica, non-root UID 65532.
- Internal Services: `ani-inference-grpc:19090` and
  `ani-inference-admin:19091`, both ClusterIP.
- Image: `docker.changqingyun.cn/ani/inference-service:ani-inference-new-20260923-r11`
  at digest
  `sha256:1be3093813a12db73309e74a971d77c36c53a45c3f249c1750f7d0c62264333a`.
- Kubernetes runtime reconciliation is enabled by default. The deployment
  uses the namespace-scoped `ani-inference` ServiceAccount, Role and
  RoleBinding to watch and manage only its owned runtime resources.
- The Inference CRD (`inferenceservices.ani.kubercloud.com`) and the LWS
  controller/CRDs are installed in the new cluster. The LWS image is mirrored
  under `docker.changqingyun.cn/mirror/lws:v0.10.0` at its immutable digest.

The Inference process starts with its Kubernetes manager and all configured
operation providers. Its ready state means the control plane is wired and the
cache is synchronized; it does not mean a model runtime has been created.

## Verification

- The new image starts and reaches the Kubernetes manager; the ServiceAccount
  can read the InferenceService CRD and create Deployments, Jobs, PVCs,
  LeaderWorkerSets and HTTPRoutes in `ani-inference`.
- The new-cluster nodes, LWS controller, Inference CRD and Gateway are ready.
- The Inference Pod is `1/1` with zero restarts; `/healthz` and `/readyz` both
  return HTTP 200.
- The local validation quota adapter is wired with a per-reservation limit of
  10 demand units. No external quota service is configured.
- The new-cluster PostgreSQL query confirmed 11 `inference_%` tables. No
  HTTPRoute, model runtime or public inference call was created.

## 2026-09-22 direct Model and fixed-quota update

The isolated validation requirement changed: Model does not require a trusted
Principal or TLS. Inference now uses plaintext Model gRPC inside the cluster,
and the materializer accepts the MinIO HTTP signed URL used by this environment
as well as HTTPS. The fetch Job receives the recorded Model artifact size as its
limit instead of the old fixed 512 MiB ceiling. The quota adapter is a local
fixed-capacity provider with a limit of 10 demand units per reservation and no
external IAM/quota dependency.

Inference was rolled to `ani-inference-new-20260923-r11` at
`sha256:1be3093813a12db73309e74a971d77c36c53a45c3f249c1750f7d0c62264333a`.
Both `ani-model-service` and `ani-inference-service` are `1/1 Running`, and
both admin `/readyz` endpoints return `{"status":"ready"}`.

## 2026-09-23 new-cluster real Model-to-Inference validation

This is the actual validation evidence for the `.clusterenv` cluster only. The
old kubeconfig/context and old namespaces were not used or modified.

### Model and artifact

- Model direct requests use tenant `11111111-1111-4111-8111-111111111111`.
  `CreateModel` succeeded for `SmolLM2-135M` and `CreateModelVersion` accepted
  the pinned CPU engine command.
- The remote import Job was exercised against
  `HuggingFaceTB/SmolLM2-135M@93efa2f097d58c2a74874c7e644dbc9b0cee75a2`.
  The new worker nodes cannot reliably reach Hugging Face (worker egress timed
  out/reset), so that Job failed before download. No model status was faked.
- The same pinned files were downloaded from the development machine, tarred,
  uploaded through `GetUploadURL` and a signed MinIO PUT to the new cluster,
  then finalized through `CreateModelVersion`. The ready version is
  `7e4b355c-9cbd-4f5e-ba98-c75e3aadedc1`, size `272455680`, artifact ref
  `11111111-1111-4111-8111-111111111111/SmolLM2-135M/93efa2f097d58c2a74874c7e644dbc9b0cee75a2-upload/model.tar`,
  SHA256 `b667a120e505d6ce9361ae266107388399f958ef9fb1afd677c191f72b6ba0a3`.
- Model service image r8 is deployed at
  `docker.changqingyun.cn/ani/model-service@sha256:95a94361021e3f02b922b18512cbde93e6d3a75c46a2bbd345b72a8acd1b7b28`.
  It is a static amd64 binary, includes the CA bundle, uses the official
  Hugging Face revision manifest endpoint with `blobs=true`, and has a 30s
  gRPC timeout for artifact finalization.

### Materialization and runtime

- Inference service `ae68574f-3127-48f3-9a30-6788684797c8`, name
  `smollm2-135m-cpu-real`, was created through plaintext gRPC with metadata
  `tenant-id`; no Model trusted identity/TLS or external IAM was used.
- Create operation `0ca12c17-f795-475a-a958-c975199d8155` reached
  `succeeded/complete` for generation 1.
- Materializer PVC `ani-model-1de4eade797ad5bf084d2ca9` is Bound on CephFS and
  Job `ani-model-1de4eade797ad5bf084d2ca9-fetch-h4t7h` completed. Its log
  verified the artifact SHA256 and `272455680` bytes with 10 files.
- Runtime Deployment Pod
  `smollm2-135m-cpu-real-59f6fbcd5f-5qsnl` is `1/1 Running` on
  `ani-k8s-worker-1`; vLLM 0.19.1 loaded `LlamaForCausalLM` from `/models`,
  used CPU mode, and passed `/health`. The mirrored runtime image is
  `docker.changqingyun.cn/mirror/vllm-openai-cpu@sha256:4c697ae650ebeb3a41f3c9c7020913d4c84d2729dc428ce39d60ca353975a4ce`.
- HTTPRoute `ani-pub-ff4311b9647ad6f1f224` is `Accepted=True` and
  `ResolvedRefs=True`, routing `/v1` to
  `smollm2-135m-cpu-real-endpoint` through the new APISIX NodePort.

### Real request

```
curl -H 'Content-Type: application/json' \
  -X POST http://192.168.102.68:30090/v1/completions \
  -d '{"model":"smollm2-135m","prompt":"The capital of France is","max_tokens":8,"temperature":0}'
```

The request returned HTTP 200 with completion id `cmpl-802a3cf16b53ee7c`,
model `smollm2-135m`, eight completion tokens and non-empty text:
`the capital of the country.`

### Fixes discovered during the run

- The materializer now accepts safe tar bundles containing a leading `./` root
  directory and normalizes those members without permitting traversal,
  symlinks or duplicate files. Regression tests cover this archive shape.
- The Inference PostgreSQL runtime source retains the previous generation's
  fenced bindings while a replacement generation has not yet applied its new
  bindings. This is required by update/delete absence handling and was tested
  with the focused Postgres package tests.
- An attempted update from a 4 CPU/8Gi test service exposed a separate
  `resource_work` stale-generation retry condition under repeated status
  notifications. That test service is not used for the successful completion;
  the successful service was created with the available cluster capacity of 2
  CPU/4Gi. It remains a follow-up for update lifecycle cleanup.

### Independent recheck

The new-cluster deployments for Model, Inference and the real CPU runtime are
all 1/1 available. Fresh gRPC reads confirm ModelVersion
`7e4b355c-9cbd-4f5e-ba98-c75e3aadedc1` is ready and create operation
`0ca12c17-f795-475a-a958-c975199d8155` is succeeded/complete.
The HTTPRoute remains Accepted=True and ResolvedRefs=True. A second completion
request returned HTTP 200, id `cmpl-aca30c8fac597fe9`, with eight completion
tokens and non-empty generated text.

The separate update operation `85939549-9d48-41e4-9e94-c289afbd1e02` still
reports pending/observe_absence, provider_retry, stale generation. Remote
ImportTask `7943b8ff-9e31-4ee4-b29c-8671f088af34` remains failed after six
attempts. Neither path is reported as passing. GPU inference is unverified;
no GPU is present in the new cluster.

### Materializer size-boundary recheck

A temporary localhost HTTP source served a 537,927,680-byte (513.0 MiB) tar
bundle. With `MODEL_MAX_BYTES` set to the recorded artifact size, the
materializer downloaded it over HTTP, verified SHA256
`6d274cf309f91f8498aa8d3fcef7925a8a456654d77c9eccbc2950f38ddc8cf1`, and
extracted all three required files successfully. The temporary source and
bundle were removed afterward. The 512Mi container memory limit is a runtime
resource setting; it is not the artifact-size ceiling. PVC capacity and the
streaming download limit use the Model version's `size_bytes`.

## 2026-09-23 larger-model validation: Qwen2.5-1.5B

The small 272,455,680-byte model is not the only runtime check. On the new
`.clusterenv` cluster, a pinned `Qwen/Qwen2.5-1.5B-Instruct` revision
`989aa7980e4cf806f80c7fef2b1adb7bc71aa306` was downloaded on the development
machine, archived and uploaded through the normal signed Model upload path.
The tar is `3,098,992,640` bytes with SHA256
`d0f3c1294ac34d9ca84858cf93bfe4dbae8ebdfa2a317d1f90970b1dd85f11dd` and has
10 model files.

- ModelVersion `515adc62-8694-490e-9e23-a6e7d436c984` is `ready`; its
  persisted `sizeBytes` is `3098992640`, so the Inference materializer can
  calculate storage and download bounds from the real artifact size.
- The materializer PVC was 6Gi on CephFS and its Job completed with
  `model bundle verified sha256=d0f3...f11dd bytes=3098992640 files=10`.
- Inference service `34f7ef2c-15f8-4a8a-89f8-e4a7a455d694`, safe Kubernetes name
  `qwen-1-5b-cpu-sizefix`, used 3 CPU/6Gi requests and 3 CPU/8Gi limits. Its
  create operation `a916fe58-17ed-4bf8-ae14-270a5f532f0e` reached
  `succeeded/complete`; vLLM CPU loaded the model with `--dtype float32` and
  `--max-model-len 512`.
- The runtime was published through the new APISIX NodePort. A real
  `POST http://192.168.102.68:30090/v1/completions` returned HTTP 200 and
  non-empty output: `Paris. The capital of Italy is Rome`.

The cluster has three 8-CPU nodes, no GPU, and only about 15Gi allocatable
memory per worker. A single 1.5B CPU runtime is therefore the practical large
model validation point on this cluster; 3B/7B runtimes need a larger-memory
worker or GPU node. Running two 1.5B attempts concurrently also demonstrated
that worker-2 capacity is tight, so future checks must be serialized.

The test exposed two follow-ups. First, a service name containing dots
(`qwen2.5-1.5b-cpu-sizefix`) is accepted by the API but cannot produce its
`<name>-endpoint` Kubernetes Service because Kubernetes Service names require
DNS-1035 labels; use hyphenated service names until admission validation or a
collision-safe name normalizer is added. Its create operation remains in
`pending/apply_runtime` with `provider_retry/stale generation`. Second, two
services currently publish the same `/v1` route, so the old small-model route
was temporarily moved to `/small-v1` for the Qwen gateway request and then
restored. Route selection needs a host/path publication policy before exposing
multiple models simultaneously.

After validation, the invalid dotted-name test Deployment was scaled to zero
on the new cluster so it does not consume worker capacity. Its API operation is
still pending because the current API has no cancellation operation and the
active create operation blocks Stop/Delete with `FAILED_PRECONDITION`; the
successful hyphenated service and the existing 135M service remain separate.
