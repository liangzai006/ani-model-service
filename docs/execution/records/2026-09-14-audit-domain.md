# Audit event domain slice

新增 `internal/biz/audit`，定义不可变审计事件字段：tenant、actor、workload、request_id、task_id、action、before/after 状态和 error class。`Validate` 强制可信调用上下文和至少一个状态/错误结果，供本地事务写入前复用。

验证：`go test ./internal/biz/audit ./internal/service`、`go vet ./...`、`go build ./...` 均通过。尚未接入事务审计写入和指标 exporter。
