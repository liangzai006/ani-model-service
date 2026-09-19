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
