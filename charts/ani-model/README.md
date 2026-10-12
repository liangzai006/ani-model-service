# ani-model

安装到已经存在且标记为新环境的 `ani-system`。一条 `helm upgrade --install` 会先跑数据库
migration Job，等 Job 成功，再部署 Model。Chart 不创建 Namespace、PostgreSQL、
S3，也不依赖 Inference。

公共 cert-manager 和 `ClusterIssuer/ani-internal` 要先就绪。Chart 默认申请 Model 服务端证书。
PostgreSQL DSN 和 S3 凭据要事先放进 Secret。直接 S3 在二进制里用 `ANI_MINIO_*`
读取。`inference.grpcAddress` 默认为空；配置它时必须同时提供 mTLS Secret。

安装命令见 [Model 安装说明](../../docs/deploy/helm/ani-model.md)。
