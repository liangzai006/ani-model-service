# Import task creation PostgreSQL slice

新增 sqlc `CreateImportTask` 查询和 `WorkStore.Create` adapter：

- tenant/model/version 复合关联由 schema 强制
- idempotency_key 使用租户内唯一约束
- 初始状态固定为 pending
- 所有关联 UUID 在 adapter 边界解析，禁止跨租户引用

ImportModel handler 尚需传入真实 model/version 关联并在同一事务中完成模型、任务和审计写入。

验证：sqlc generate、`go test ./internal/data/postgres ./internal/biz/work`、`go vet ./...`、`go build ./...` 均通过。
