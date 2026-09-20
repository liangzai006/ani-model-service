# Historical MinIO provisioning command

日期：2026-09-15

- 初版 `cmd/minio-provision` 只在显式执行时检查并创建 Model bucket；后续已将同一入口改为 Kubernetes Import Job 的一次性导入命令。
- 默认 bucket 为 `ani-models`，可用 `ANI_MINIO_BUCKET` 覆盖。
- 设置 `ANI_MINIO_TENANT_ID` 时，命令将该 UUID 转换为同名租户 bucket；运行时
  `ANI_MINIO_TENANT_BUCKETS=true` 时使用同一命名规则。
- endpoint、access key、secret key 和 secure 模式均来自环境变量；生产环境通过 Kubernetes Secret 的 `EnvFrom` 注入 Job。
- bucket 检查/创建保持幂等；现在 bucket 成功后命令继续执行 provider CLI 下载、归档和上传。

## 未执行

已在集群执行一次显式 provisioning：使用 `ani-s05-objectstore/ani-s05-minio` 的 NodePort、Secret `ani-s05-minio-root` 注入的凭据，成功创建 `ani-models` bucket。没有修改 PVC、Deployment 或其他集群资源。

旧的显式 provisioning 记录只说明 bucket 操作，不能作为独立 Import Job 的集群验收证据。
