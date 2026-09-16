# MinIO S3 adapter

日期：2026-09-15

## 实现

- 新增 `internal/data/storage/MinIOAdapter`，实现 Storage port 的上传签名、对象存在性、SHA256 校验和短期下载地址。
- 对象 key 强制按 tenant 前缀归属；跨 UUID tenant 前缀被拒绝。
- worker 使用 `PutObject` 流式写入，不把模型文件整体读入内存。
- Model 启动时只读检查已配置 bucket 和凭据；bucket 不存在或凭据无效时启动失败，不自动创建 bucket。
- 通过 `ANI_MINIO_ENDPOINT`、`ANI_MINIO_ACCESS_KEY`、`ANI_MINIO_SECRET_KEY`、`ANI_MINIO_BUCKET`、`ANI_MINIO_SECURE` 配置。
- `ANI_STORAGE_GRPC_ADDR` 与 MinIO 配置互斥，避免同一进程选择不确定的 Storage 后端。

2026-09-16 补充：设置 `ANI_MINIO_TENANT_BUCKETS=true` 后 adapter 使用 tenant UUID
作为 bucket 名称，并在远程导入下载前检查、按需创建 bucket；共享 bucket 行为保留兼容。

## 验证

- 集群 MinIO readiness：`http://10.10.1.68:30900/minio/health/ready` 成功。
- MinIO adapter tenant key、配置校验和 Storage port 编译测试通过。
- 真实对象写入、存在性、内容 SHA256 和预签名下载地址验证已在端到端导入中通过；1 秒 TTL 地址等待 2 秒后返回 HTTP 403。

## 未验证

- 集群 Secret 仅通过临时环境注入使用，未写入仓库。
- bucket 初始化仍需独立 provisioning 步骤；Model 不在启动时创建 bucket。
