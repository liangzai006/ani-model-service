# ani-model-service

Module: `github.com/liangzai006/ani-model-service`

This repository was generated from ANI's pinned Kratos layout. It is an
independent source snapshot: builds and runtime do not require the layout.

`THIRD_PARTY_NOTICES.go-kratos-layout.txt` preserves the upstream template's
MIT notice. This generated repository intentionally has no project `LICENSE`;
its owner must make that choice before publication.

## Local commands

```bash
make tools
make verify
go run ./cmd/ani-model-service -conf ./configs
```

生产导入使用独立的 Kubernetes Job，不占用 Model Pod 的本地磁盘。Model 服务先把任务
写入 PostgreSQL，再创建一个带大容量 PVC 的 Job；Job 以
`/ani-model-import`（`cmd/minio-provision`）为入口，先检查并幂等创建目标 bucket，
然后调用 ModelScope/Hugging Face CLI 下载到 PVC，打包后上传 MinIO。Job 完成后，Model
worker 校验对象和 SHA-256，再写入 artifact 并把版本置为 ready。导入日志写到 Job
标准输出，包含 provider、任务和阶段信息；不会输出凭据或签名 URL。查看方式：

```bash
# 本地
go run ./cmd/ani-model-service -conf ./configs 2>&1 | tee model-service.log

# Kubernetes
kubectl -n <import-namespace> get jobs,pods -l ani.liangzai006.io/import-job=true
kubectl -n <import-namespace> logs job/<job-name> -c import -f
```

真实导入成功后，`public.model_artifacts.reference` 保存 Storage 对象引用，
`GetModelDownloadURL` 只返回短期签名地址；对象本身要在 Storage 的 bucket 中查看。

After the initial source commit, run `make supply-chain-tools` and `make audit`;
review and commit `docs/scaffold/bom.cdx.json`. CI deliberately fails when that
runtime SBOM is missing or stale and reruns the vulnerability, secret,
notice-integrity, and license-evidence gates.

The committed listeners are loopback-only local defaults. Override them through
the typed `ANI` environment configuration when the deployment design is added.

## Runtime shell

Business requests follow `Client --HTTP--> independent Gateway --gRPC--> Model`; Model performs the model business processing.
The Gateway lives outside this repository and owns the public HTTP entry point
and HTTP-to-gRPC forwarding. Model uses the pinned layout's Kratos gRPC transport.
Its separate admin HTTP listener serves only `/healthz`, `/readyz`, and `/metrics`;
it is not a business API or a Gateway.

Storage integration must use the external service's confirmed, versioned contract.
The earlier speculative HTTP adapter has been withdrawn; the Storage domain port
remains, and real Storage integration is `not_verified`.

Model writes only the `public` schema. Keep the Model and Inference database
credentials separate in the deployment configuration.

When MinIO is used directly, set `ANI_MINIO_ENDPOINT`,
`ANI_MINIO_ACCESS_KEY`, and `ANI_MINIO_SECRET_KEY`; the credentials should be
injected from a Kubernetes Secret. `ANI_MINIO_BUCKET` defaults to
`ani-models` for shared-bucket compatibility. With
`ANI_MINIO_TENANT_BUCKETS=true`, each tenant uses a bucket named by its UUID;
otherwise the shared bucket stores objects under the tenant prefix. The Job
checks and creates the selected bucket immediately before downloading.

Set `ANI_IMPORT_EXECUTION_MODE=kubernetes` on the Model Deployment and provide
`ANI_IMPORT_JOB_IMAGE` and `ANI_IMPORT_KUBERNETES_NAMESPACE`. You may omit
`ANI_IMPORT_STORAGE_CLASS`; Kubernetes then uses its configured default
StorageClass. You may also omit `ANI_IMPORT_STORAGE_SIZE`: the worker sums the
provider manifest and adds headroom before creating the PVC. If the provider
does not expose sizes (for example a private manifest that the Model Pod
cannot read), the task asks for an explicit size instead. The Model Pod's
service account needs permission to create/get Jobs and PVCs and read Job Pods'
logs. `ANI_IMPORT_MINIO_SECRET` supplies MinIO credentials to the Job and
`ANI_IMPORT_PROVIDER_SECRET` may supply `MODELSCOPE_API_TOKEN` or Hugging Face
credentials. See [the Job deployment example](docs/deploy/import-job.yaml).

The PostgreSQL-backed worker scans due tasks across tenants and passes each
task's real tenant ID to the Job. The Deployment does not contain a tenant UUID.

The import image must contain the compiled command at `/ani-model-import`, the
`modelscope` CLI, and optionally the `hf` CLI. `Dockerfile.import` is a minimal
image example. The command streams the tar upload and does not impose the old
512 MiB in-process limit; the available PVC and MinIO capacity remain the
physical limits.

- Kratos lifecycle with graceful shutdown.
- gRPC and a separate admin HTTP server.
- Structured redacted logs, tracing, metrics, and middleware.
- `/healthz` reports process liveness.
- `/readyz` reports only completion of the local runtime start hook; add checks
  for real dependencies when the first vertical slice introduces them.
- `/metrics` exports the local Prometheus registry.

Domain, persistence and handler code is in progress. Production dependency wiring
and real integration evidence are tracked in `docs/execution/status.md`.
