# ani-inference

此 Chart 描述新环境中的 Inference 控制面。它负责创建和观察 KServe
InferenceService/LLMInferenceService 及 LeaderWorkerSet 资源，并通过新环境
Higress 网关发布模型路由。Chart 不安装 KServe、LWS、Gateway API 或 Higress
Operator，也不会生成模型引擎启动命令。

安装时必须设置 `higress.publicBaseURL`，并确保它指向新环境。Chart 会创建
命名空间级运行时 RBAC；启用相关配置后，还会创建跨命名空间权限，用于更新
Higress 限流 WasmPlugin。安装顺序见 [Chart 总说明](../README.md) 以及
[新环境 Helm 部署文档](../../docs/deploy/helm/ani-services.md)。
