# Explicit MinIO bucket provisioning

日期：2026-09-15

- 新增 `cmd/minio-provision`，只在显式执行时检查并创建 Model bucket。
- 默认 bucket 为 `ani-models`，可用 `ANI_MINIO_BUCKET` 覆盖。
- 设置 `ANI_MINIO_TENANT_ID` 时，命令将该 UUID 转换为同名租户 bucket；运行时
  `ANI_MINIO_TENANT_BUCKETS=true` 时使用同一命名规则。
- endpoint、access key、secret key 和 secure 模式均来自环境变量；生产环境通过 Kubernetes Secret 的 `valueFrom.secretKeyRef` 注入。
- 命令幂等：bucket 已存在时直接成功；Model 服务启动路径不会调用该命令。

## 未执行

已在集群执行一次显式 provisioning：使用 `ani-s05-objectstore/ani-s05-minio` 的 NodePort、Secret `ani-s05-minio-root` 注入的凭据，成功创建 `ani-models` bucket。没有修改 PVC、Deployment 或其他集群资源。

真实导入仍需 Model 服务使用同一 endpoint、bucket 和 Secret 注入方式启动，并完成对象上传、checksum 和短期下载验证。
