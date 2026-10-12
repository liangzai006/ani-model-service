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
写入 PostgreSQL，再通过当前 Storage Port 生成任务级签名上传地址，并创建带 PVC 的
Job。Job 只接收签名地址、任务元数据和 provider 配置，不接收 MinIO/S3 endpoint、
bucket 或访问密钥；下载完成后流式上传模型归档。Model worker 校验对象存在性、大小
和 SHA-256，再写入 artifact 并把版本置为 ready。导入日志写到 Job 标准输出，不会
输出凭据或签名 URL。查看方式：

```bash
# 本地
go run ./cmd/ani-model-service -conf ./configs 2>&1 | tee model-service.log

# Kubernetes
kubectl --context "$KUBE_CONTEXT" -n <import-namespace> get jobs,pods \
  -l ani.liangzai006.io/import-job=true
kubectl --context "$KUBE_CONTEXT" -n <import-namespace> logs job/<job-name> -c import -f
```

真实导入成功后，`public.model_artifacts.reference` 保存 Storage 对象引用，
`GetModelDownloadURL` 只返回短期签名地址；对象本身要在 Storage 的 bucket 中查看。
新环境从零部署请按 [Model Helm 部署指南](docs/deploy/helm/ani-model.md) 执行。

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

当前默认 Storage 是 S3 兼容适配器，因此服务端使用 `ANI_MINIO_ENDPOINT`、
`ANI_MINIO_ACCESS_KEY` 和 `ANI_MINIO_SECRET_KEY` 连接对象存储；变量名称沿用现有
实现，并不限定后端必须是 MinIO。`ANI_MINIO_BUCKET` 默认为 `ani-models`，启用
`ANI_MINIO_TENANT_BUCKETS=true` 后按租户 UUID 使用独立 bucket，否则使用共享 bucket
和租户前缀。也可以通过 `ANI_STORAGE_GRPC_ADDR` 选择 Storage gRPC 适配器，两个模式
不能同时配置。

导入始终使用 Kubernetes Job。必须提供 `ANI_IMPORT_JOB_IMAGE` 和
`ANI_IMPORT_KUBERNETES_NAMESPACE`；Job 使用 Model Deployment 的
`ani-model-service` ServiceAccount。可选 `ANI_IMPORT_PROVIDER_SECRET` 注入
ModelScope/Hugging Face provider 凭据，可选 `ANI_IMPORT_STORAGE_CLASS` 指定 PVC
StorageClass，省略时使用集群默认值。PVC 大小由 provider manifest 和模型实际大小
计算，代码不接受手工的存储大小或引擎参数覆盖。Job 的 ServiceAccount、执行方式
和存储地址均由当前服务契约统一管理。

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
- `/readyz` reports PostgreSQL、Storage 和持久 worker 等已配置依赖的就绪状态。
- `/metrics` exports the local Prometheus registry.

Domain, persistence and handler code is in progress. Production dependency wiring
and real integration evidence are tracked in `docs/execution/status.md`.
