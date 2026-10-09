# ani-model

此 Chart 描述新环境中的 Model 服务及其 Kubernetes 导入控制器。控制器使用固定
的 `ani-model-service` ServiceAccount 创建短生命周期导入 Job；这个名称属于
当前应用契约，不会按 Helm release 改名。

默认存储适配器连接新环境中的 S3 兼容服务。Chart values 使用与 provider 无关
的 `storage` 名称，虽然当前二进制会把直接 S3 配置映射为 `ANI_MINIO_*`
环境变量。导入 Job 的存储大小不是 Chart 参数，服务会根据模型 manifest
计算物化所需容量。

部署文档会在安装 Chart 之前创建数据库 Secret、S3 凭据 Secret 和 CA ConfigMap。
安装顺序和 migration 顺序见 [Chart 总说明](../README.md) 以及
[新环境 Helm 部署文档](../../docs/deploy/helm/ani-services.md)。
