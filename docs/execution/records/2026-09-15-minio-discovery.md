# MinIO dependency discovery

日期：2026-09-15

## 发现

- 集群存在 `ani-s05-objectstore/ani-s05-minio` NodePort Service。
- S3 API：Service 端口 `9000`，NodePort `30900`；console 端口 `9001`。
- EndpointSlice 显示后端 `10.16.23.41` ready，开发机访问 `http://10.10.1.68:30900/minio/health/ready` 成功。
- MinIO 镜像为 `minio/minio:RELEASE.2025-04-22T22-12-26Z`。
- 凭据只读确认来自 Secret `ani-s05-minio-root` 的 `access_key_id`、`secret_access_key`，未读取 Secret 值。
- 未发现独立 Storage gRPC Service；bucket 名称和 Model 使用的认证注入方式仍待确认。

## 边界

后续 MinIO adapter 只调用已存在的 S3 bucket 和对象 API，不创建 bucket、不管理 PVC、不接管 MinIO 生命周期。Model 仍通过 Storage 领域 port 使用该 adapter。

## 未验证

真实 bucket、凭据注入、presigned URL、对象 checksum 和导入任务端到端流程仍为 `not_verified`。
