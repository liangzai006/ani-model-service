# Helm 部署文档入口已拆分

Model 的新环境部署指南已移到
[ani-model.md](ani-model.md)，本文保留为旧链接的迁移入口。

Inference 属于独立仓库，请使用其仓库内的
`docs/deploy/helm/ani-inference.md` 中的 Helm 和 `kubectl apply` 命令。以仓库主页为入口：
[ani-inference-service](https://github.com/liangzai006/ani-inference-service)。
Inference 仓库的安装文档负责说明 KServe、LWS、Gateway API、Higress 及
Inference Chart；本仓库不再提供或安装 Inference Chart，也不把这些
平台依赖、PostgreSQL 或 S3 打包为 Model 的 Helm 依赖。
