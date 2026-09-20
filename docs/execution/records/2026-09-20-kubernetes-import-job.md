# Kubernetes Import Job implementation record

日期：2026-09-20

这次改动把 `cmd/minio-provision/main.go` 从只做 bucket provisioning 的命令改成独立
Import Job 的入口。它按以下顺序执行：

1. 根据 `ANI_MINIO_TENANT_BUCKETS` 选择共享 bucket 或租户 UUID bucket。
2. 调用 `BucketExists`，缺失时调用 `MakeBucket`，并处理并发创建竞态。
3. 按 `ANI_IMPORT_SOURCE` 运行 `modelscope download` 或 `hf download`，目标是 Job 的 staging PVC。
4. 将 staging 目录流式打成 `imports/<task-id>/model.tar`，上传 MinIO，计算 SHA-256，并输出
   `ANI_IMPORT_RESULT` 结果行。

Model worker 在 `ANI_IMPORT_EXECUTION_MODE=kubernetes` 时创建 PVC 和 Job。Job 名称由 task
和 attempt 组成：同一 attempt 在 worker 重启后复用，不同 attempt 使用新 Job/PVC。成功后
worker 读取结果行，校验对象存在性和 checksum，再写入 artifact 并完成原有 ready/CAS 流程。

Deployment 不配置租户 UUID。PostgreSQL worker 扫描所有租户的 due task，并使用每条任务
自身的 `tenant_id` 完成 claim、bucket 和 Job 环境注入。

`ANI_IMPORT_STORAGE_CLASS` 为空时，PVC 的 `storageClassName` 保持为空，由 Kubernetes
选择集群默认 StorageClass。`ANI_IMPORT_STORAGE_SIZE` 为空时，Model worker 读取 provider
manifest 的文件大小，按总大小增加 20% 且至少增加 10Gi，再向上取整到 Gi；manifest 无法
读取或存在未知文件大小时拒绝创建 Job，要求显式配置容量，避免为大模型创建不足的 PVC。

本地证据：

- `go test -count=1 ./...`
- `go vet ./...`
- `go build -trimpath ./...`
- `git diff --check`

尚未声称完成的验收：真实 Kubernetes API 创建 Job、真实 ModelScope 大模型下载、真实大容量
PVC/MinIO 上传，以及后续 Inference materializer 的完整联调。`docs/deploy/import-job.yaml`
包含 RBAC、Secret 和 Model Deployment 环境变量示例。
