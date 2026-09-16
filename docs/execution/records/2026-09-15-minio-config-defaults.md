# MinIO configuration defaults

日期：2026-09-15

- `ANI_MINIO_BUCKET` 缺省为 `ani-models`，这是配置默认值，不是业务固定模型或 provider。
- `ANI_MINIO_ACCESS_KEY`、`ANI_MINIO_SECRET_KEY` 只从 Kubernetes Secret 通过 `valueFrom.secretKeyRef` 注入环境变量。
- Secret 值不写入仓库、不打印日志；bucket 由独立 provisioning 创建。
- 共享 bucket 模式下 Model 启动只读检查 bucket；租户 bucket 模式下导入任务在下载前按 tenant UUID 检查并幂等创建目标 bucket。
- `docs/deploy/minio-env.yaml` 给出集群 Service 和 Secret 的环境注入片段；它不是可直接 apply 的完整 Deployment。
