# Readiness dependency gate slice

扩展 `Readiness` 支持依赖门控：启用 `RequireDependencies` 后，PostgreSQL、Storage、worker 三项必须分别报告 ready，任何一项缺失均保持 `/readyz` 未就绪。默认仍保持骨架的进程级行为，便于现有生命周期测试；正式组合根接入依赖后应启用门控。

验证：`go test ./internal/server`、`go vet ./...`、`go build ./...` 均通过。
