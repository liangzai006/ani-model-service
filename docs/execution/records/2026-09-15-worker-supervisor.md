# Durable worker lifecycle supervisor

日期：2026-09-15

## 实现

- 新增 `server.WorkerSupervisor`，通过 `Start`/`Stop` 管理持久导入 worker 的 context 和退出信号。
- worker loop 启动后才设置 `worker` readiness；worker 返回（无论错误或正常退出）都会清除 readiness。
- `cmd/ani-model-service` 的 app composition root 支持可选注入 `WorkerRunner`；未提供 runner 时，依赖门控服务保持未就绪。
- supervisor 不创建队列或存储资源，也不提供默认 provider；真实 HuggingFace/ModelScope 配置仍由后续外部 adapter 切片接入。

## 验证

- `go test ./internal/server ./cmd/ani-model-service` 通过。
- 开发机完整 `make verify` 通过：Buf 固定版本、生成稳定性、`go mod tidy -diff`、全量测试、vet、build、mod verify 和 diff check。

## 未验证

- 真实 provider registry、worker 进程崩溃恢复和外部 Storage 上传联调仍为 `not_verified`。
