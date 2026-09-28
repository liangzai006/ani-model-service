# ModelScope + KServe 0.5B 真实推理验证 — 2026-09-24

## 范围

本次只使用新集群上下文 `kubernetes-admin@kubekey`（API
`https://192.168.102.68:6443`），旧集群、旧项目和既有推理服务未操作。
验证目标是把已经导入 MinIO 的 ModelScope 制品接入 KServe RawDeployment，并从
Higress 固定入口完成一次真实生成。

## 验证结果

- ModelVersion 为
  `9e402032-f244-4826-9952-3fbbb6edc22e`，制品大小 `999602688` 字节，SHA256 为
  `e1fc761d399f82372857717c6b38c87ad238f9dc85cf033111b7b2d07bb89e1c`。
- 创建的推理服务为 `qwen2-0-5b-ms-kserve`，service ID 为
  `1c5dd2cf-785c-443a-936f-4d95814ce0e4`，创建 operation
  `aaae799e-f23a-45df-bf5f-3ba5f50f4317` 最终为 `succeeded/complete`。
- KServe predictor 使用 `4 CPU / 8Gi`，在 worker-2 上 Ready；物化 Job 日志确认
  `999602688` 字节、`11` 个文件和上述 SHA256 均匹配。
- HTTPRoute 为 Accepted/ResolvedRefs，backend 指向 KServe predictor，固定路径仍为
  `/v1/completions`，模型别名为 `qwen2-0.5b-instruct-ms`。
- 通过新集群 Higress NodePort
  `http://192.168.102.68:30090/v1/completions` 发送仅包含 JSON
  `{"model":"qwen2-0.5b-instruct-ms",...}` 的请求，未手工添加模型路由 Header，
  返回 HTTP 200 和非空 completion。说明 Higress model-router 能从请求体的
  `model` 字段完成分流；冷请求约 34 秒。

## 清理与边界

验证完成后通过业务 Delete 完成删除 operation，并按该 service ID 精确清理 KServe
资源、HTTPRoute、运行时绑定、物化 Job/PVC/Secret。ModelScope 的 MinIO 制品和
`ModelVersion ready` 记录保留；worker-2 恢复到约 `310m CPU / 490Mi` 请求，只保留
已验证的 `smollm2-135m-cpu-real` 常驻推理服务。

当前新集群无 GPU；更大的模型需要继续串行验证并先确认 worker-2 容量，不能同时占用
多个大模型副本。
