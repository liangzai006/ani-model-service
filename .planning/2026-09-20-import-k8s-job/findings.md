# 调查发现

- `cmd/ani-model-service/main.go` 当前只组装 `worker.Worker`，没有 Kubernetes client 或 `batch/v1.Job`。
- `internal/service/model.go:435` 的 `ImportModel` 只落 PostgreSQL 任务并通知 in-process worker。
- `internal/worker/import_executor.go` 直接 HTTP 下载并上传 Storage，完整仓库还打成 tar，存在 512 MiB 限制。
- Inference 的 `model-fetch` Job 是模型 ready 后的物化步骤，不能复用为导入 Job。
- ModelScope 官方 CLI 支持 `modelscope download --model ... --revision ... --local_dir ...`；新版 `ms-hub` 支持续传/并行/重试能力，但镜像必须固定版本并实测。
- ModelScope CLI 只写本地目录；几百 GB 场景需要大容量 staging PVC 和分片/并行上传 Storage。
