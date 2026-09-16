# PostgreSQL import lease adapter slice

新增 `internal/data/postgres/work_store.go`，封装 sqlc 的 Claim/Complete/Retry 查询。所有入口先解析并绑定 `tenant_id` 与 task UUID；完成和重试使用 owner + lease_epoch 的 CAS 条件，影响行数不是 1 时返回 `ErrLeaseLost`，阻止旧 worker 写回。

验证：`go test ./internal/data/postgres ./internal/biz/work`、`go vet ./...`、`go build ./...` 均通过。尚未执行真实 PostgreSQL worker 重启演练。
