# Inference ModelVersion 切换实现与验证

日期：2026-09-18。D 切片先完成版本切换这一条独立路径。

## 已实现

- `UpdateInferenceServiceRequest` 增加 `model_version_id`，field mask 接受该字段。
- Inference Update 通过已有版本化 Model gRPC `GetModelVersion` 读取目标版本；目标必须 ready、版本 ID 和 artifact 摘要完整且一致。
- 目标版本的 artifact reference、SHA256、engine runtime 和启动命令由 Model 快照写入下一代 Inference spec；不允许只传 `model_artifact` 绕过 Model 版本校验。
- 没有指定新版本的资源/副本更新保留旧版本快照。
- 下一代 spec 仍按现有 generation CAS、quota、publication 和 runtime worker 流程处理；物化阶段继续以 Inference spec 的版本 ID 再次读取 Model。

## 验证

- Model adapter 定向测试：ready 快照映射、未 ready 版本拒绝。
- Inference service、PostgreSQL、Kubernetes 定向测试通过。
- Inference 实际仓库：`go test -p 2 ./... -count=1`、`go vet -p 2 ./...`、`go build -p 2 ./...`、`/tmp/ani-buf lint` 和 `git diff --check` 通过。
- 真实 PostgreSQL `TestUpdateUseCaseClonesSpecWithGenerationCAS` 通过，确认新版本 UUID 写入 generation 2 spec，旧 generation 保留。

## 尚未闭合

这条记录不代表 D 完成。通用引擎/容量限制、非 vLLM/分布式 runtime、真实 create/ready/stop/start/restart/update/delete 生命周期和进程/Pod 恢复仍待专用 Kubernetes 验收。
