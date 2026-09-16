# Gateway 入口边界纠正

本轮切片依据用户明确的部署架构：客户端 HTTP → 独立 Gateway → gRPC → Model。

- 核对 `cmd/ani-model-service/app.go` 和 `internal/server/admin.go`：Model 业务注册于
  Kratos gRPC；admin HTTP 只注册 healthz、readyz、metrics。
- 本仓库不实现 Gateway，也不新增业务 HTTP 路由。
- Storage HTTP adapter 的路径未根据正式外部契约核验，而且没有运行调用方。
  将其移动为 `archive/2026-09-14-storage-http.go.txt` 历史文本，保留内容以便追溯，
  从 Go 编译源码中撤出。保留现有 Storage 领域 port。
- 更新 README、设计、计划和 status，纠正“HTTP adapter 是正式契约”的表述。
- Gateway 真实转发、IAM 委托校验及 Storage 真实调用继续为 `not_verified`。

本轮验证（使用 `/tmp/ani-model-*` 下的可写 Go 缓存）：

- `go test -count=1 ./internal/...`：通过；无测试文件的包仅完成编译。
- `go build ./...`：通过。
- 核对运行源码：无 `HTTPAdapter` 和四个自行假定的 Storage HTTP 路径；
  Model gRPC 注册及三个运维 HTTP 路由保留。
- `git diff --check`：通过；本仓库文件当前未跟踪，该命令不代表完整源码核验。

本轮没有执行 Gateway/Storage 联调、数据库操作、commit 或 push。

## Confirmed operating flow

用户确认：Model 负责模型业务处理，Gateway 只接收外部 HTTP 并通过 Model gRPC 调用业务；Model 不提供业务 HTTP 路由。Model 到 Storage 的调用必须使用 Storage 已确认的外部 gRPC/API 契约。
