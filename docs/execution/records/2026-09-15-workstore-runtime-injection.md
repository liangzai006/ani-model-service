# WorkStore runtime injection

日期：2026-09-15

## 实现

- `cmd/ani-model-service` 在 PostgreSQL 连接成功后构造 `postgres.WorkStore`。
- Model 服务通过现有 `NewModelServiceWithDependencies` 接收该 WorkStore，`ImportModel` 现在可以将任务写入 `public.model_import_tasks`。
- 未配置 PostgreSQL 时仍不创建内存替代队列；Model 保持依赖不可用，避免任务丢失。
- worker 的实际 provider registry 和生命周期实例化仍需外部 HuggingFace/ModelScope 与 Storage 配置后接入。

## 验证

- `go test ./cmd/ani-model-service ./internal/service ./internal/data/postgres` 通过。
- 开发机完整 `make verify` 通过。

## 未验证

- 尚未以真实运行进程提交 ImportModel 并由真实 provider worker 执行完成；该链路仍为 `not_verified`。
