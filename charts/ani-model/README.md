# ani-model

安装到已经存在且标记为新环境的 `ani-system`。一条 `helm upgrade --install` 会先跑数据库
migration Job，等 Job 成功，再部署 Model。Chart 不创建 Namespace、PostgreSQL、
S3，也不依赖 Inference。

公共 cert-manager 和 `ClusterIssuer/ani-internal` 要先就绪。Chart 默认申请 Model 服务端证书。
PostgreSQL DSN 和 S3 凭据要事先放进 Secret。直接 S3 在二进制里用 `ANI_MINIO_*`
读取。`inference.grpcAddress` 默认为空；配置它时必须同时提供 mTLS Secret。

安装命令见 [Model 安装说明](../../docs/deploy/helm/ani-model.md)。

## 离线平台接入

Chart 0.1.2 为服务、迁移和导入镜像分别提供 `image.digest`、
`migration.image.digest`、`import.jobImage.digest`。填写 `sha256:` 加 64 位小写
十六进制摘要时，生成 `repository@digest`；留空则继续使用原来的 `repository:tag`。
镜像及迁移客户端都须提前放入目标集群可用的离线仓库。

`database.existingSecret` / `secretKey` 只供服务读取运行 DSN。可用
`database.migrationSecret` / `migrationSecretKey` 为迁移 Job 提供独立 DDL 账号；
两项分别为空时，回退到原来的 Secret 名和 key，保持已有配置兼容。

复用 ANI installer 的 PostgreSQL 17、RustFS、`ani-block` 和 `ClusterIssuer/ani-ca`
时，通过现场覆盖 values 选择地址、已有凭据 Secret、
`tls.certificate.issuer.name=ani-ca`、`import.storageClass=ani-block`。
RustFS 开启原生 HTTPS，需要 `storage.direct.secure=true` 和可信 CA；endpoint
只填写 `host:port`。`inference.grpcAddress` 保持空即可单独运行 Model。
这些参数映射不表示新环境的数据库迁移、S3 兼容性或 Governance 接入已经通过验收。
