# 新集群资源释放 — 2026-09-24

本次只操作新集群上下文 `kubernetes-admin@kubekey`，旧集群和旧项目未触碰。

为给后续 ModelScope/KServe 大模型串行验证腾出容量，按 service-id 精确清理了以下
测试推理服务及其 Deployment、Service、HTTPRoute、PVC、Job、Secret 和运行时绑定：

- `qwen-1-5b-cpu-sizefix`
- `qwen-1-5b-higress-20260923`
- `qwen-1-5b-higress-live`
- `qwen2.5-1.5b-cpu`
- `qwen2.5-1.5b-cpu-sizefix`
- `smollm2-135m-cpu`

已验证的 `smollm2-135m-cpu-real` 保留运行。新集群 `worker-2` 的调度请求从约
`8682Mi/3.31 CPU` 降至约 `490Mi/0.31 CPU`，当前没有 Pending 推理 Pod。

ModelScope 导入任务已经完成，删除其已完成 Job、Pod 和 `Unused=True` 的 11Gi
staging PVC；MinIO 中的模型制品和 `ModelVersion ready` 记录保留。
