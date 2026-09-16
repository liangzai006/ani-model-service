# Runtime PostgreSQL readiness wiring

正式 `run` 现在使用共享的 `ANI_DATABASE_DSN` 创建并 Ping pgxpool；成功后注入 public schema 的 Model/Version/Catalog store。正式组合启用依赖门控：

- PostgreSQL Ping 成功才设置 postgres ready
- Storage 尚未配置
- 持久 worker 尚未配置
- 因此当前正式 `/readyz` 保持未就绪，符合依赖缺失规则

`buildApp` 测试辅助仍使用非门控模式，避免纯生命周期测试依赖外部服务。

验证：`go test ./cmd/ani-model-service -run '^$'`、`go vet ./...`、`go build ./...` 均通过。
