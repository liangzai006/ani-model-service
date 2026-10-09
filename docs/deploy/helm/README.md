# Helm 部署

Model 和 Inference 两个业务服务的完整部署步骤见
[ani-services.md](ani-services.md)。

该文档只针对 `.clusterenv` 对应的新集群，所有安装命令都显式使用
`kubernetes-admin@kubekey` context；旧集群、基础组件和 CRD 不在这两个 Chart
的管理范围内。
