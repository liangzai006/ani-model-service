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

## 模型版本与推理启动配置

`ModelVersion` 保存推理默认配置：`engine_type`、`engine_image`、完整的
`startup_command`，以及成对的 `recommended_image_id`、`recommended_image_scope`。
scope 为 `platform` 或 `tenant`。新表单从镜像目录选择，Gov 查询目录后填入固定
`engine_image` 引用；历史记录和旧调用仍可同时省略两个推荐字段。例如：

```json
{
  "engine_type": "vllm",
  "engine_image": "registry.example/vllm@sha256:<digest>",
  "recommended_image_id": "<registered-image-id>",
  "recommended_image_scope": "platform",
  "startup_command": "vllm serve /models --served-model-name qwen2.5-1.5b-instruct"
}
```

`GetModelVersion` 和 `ListModelVersions` 会原样返回这些字段。前端选择版本后将其
带入推理创建表单。使用目录的推理请求显式提交最终 `image_id`、`image_scope` 和
完整引擎命令，Inference 在接受前解析实际执行镜像；单次覆盖不会修改模型默认值。
`startup_command` 始终是一条完整字符串；服务不会把它拆成数组，也不会拼接启动
参数。

版本创建、整包导入及训练产物注册均保存推荐字段，并把选择计入现有幂等指纹；
两个字段为空时保留旧指纹。`PrepareTrainedArtifact` 继承源版本时将推荐字段与
镜像引用冻结进注册会话，重放及 `CompleteTrainedArtifact` 使用冻结值。旧 ModelDev
仍可不传新字段，源版本没有目录选择时不会伪造 ID。

`IMG-KNOWN-P2-001` 保持 **DEFERRED**：Model 已保存但响应丢失后，Gov 保存前的
目录查询仍可能因目录变化或不可用而阻断原请求重放；本期未增加补偿入口或账本。

Model、ModelVersion 列表统一使用 `page`、`pageSize`，返回 `items` 和 `total`。

## 外部模型搜索与导入

前端可以通过 Gov 的 `GET /api/v1/models/search` 按名称搜索
ModelScope 或 Hugging Face。Model 服务内部对应 `SearchExternalModels` gRPC，
只接受固定的 `source`（`modelscope` 或 `huggingface`）、`keyword`，以及可选的
`owner`、`task`、`library`、`page` 和 `pageSize`。搜索结果只作为临时候选返回，不写入本地
Model/ModelVersion，也不下载模型；ModelScope 默认 revision 是 `master`，Hugging
Face 默认 revision 是 `main`。搜索响应返回 `items` 和 `hasMore`，不承诺 `total`。

用户确认候选后，前端按 `CreateModel` → `CreateModelVersion` → `ImportModel` 的
顺序提交：

1. `CreateModel` 使用候选 `repo_id` 作为 `model_id`，建立租户本地 Model。
2. `CreateModelVersion` 使用候选 revision 建立版本，并提交调用方选择的
   `engine_type`、`engine_image` 和完整 `startup_command`。搜索逻辑不会猜测或
   拼接引擎命令。
3. `ImportModel` 携带候选 `source`、`repo_id`、`revision`，以及前两步返回的本地
   `model_id`、`model_version_id`，导入整个仓库。

取消确认不会创建记录或启动下载。导入启动后仍通过 `GetImportTask` 查询任务状态。
ModelScope/Hugging Face 的 HTTP 访问只存在于 Model 服务的 provider 适配器中，Gov
只负责鉴权、租户上下文和字段转发。

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
image example. Import Jobs run as UID/GID 65532, with `HOME=/staging` and
`fsGroup=65532`; the image and staging storage must support this identity. The
command streams the tar upload and does not impose the old
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
