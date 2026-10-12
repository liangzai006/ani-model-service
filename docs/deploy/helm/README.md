# Helm 部署

Model 的安装说明：[ani-model.md](ani-model.md)。

安装前显式核对当前 kubeconfig context 是新集群；正式 Helm 命令不传
`--kube-context`。配置 values 后执行 `helm upgrade --install`。
Helm 先初始化数据库，再部署 Model。Model 不依赖 KServe、LWS、Gateway 或 Higress；
PostgreSQL、S3 和 cert-manager 由平台公共流程提供。
