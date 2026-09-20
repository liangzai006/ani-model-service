# 进度

2026-09-20：确认现状不满足“导入任务独立 Kubernetes Job”要求；建立本阶段计划，等待 adapter/Job 契约实现。

- 2026-09-20：`cmd/minio-provision` 已改为 Job 入口：先按共享 bucket/租户 bucket 规则幂等检查并创建 bucket，再运行 ModelScope/Hugging Face CLI，流式打包并上传 `imports/<task>/model.tar`，输出带 SHA-256 的稳定结果行。
- 2026-09-20：Model worker 增加 Kubernetes 执行模式。它创建 PVC 和独立 Job，按 task + attempt 使用稳定名称，重启会复用同一尝试，重试会创建新尝试；完成后校验对象、checksum、artifact 和 version ready。
- 2026-09-20：补充 import 镜像 Dockerfile、RBAC/Secret 配置和 README。真实集群 Job、真实 ModelScope 大模型和 Inference 物化尚未在本阶段验收。

- 2026-09-20：根因确认完成；当前实现是 in-process worker，Model 仓库无 batch/v1 Job。开始设计 Kubernetes Job executor 与状态回收。
