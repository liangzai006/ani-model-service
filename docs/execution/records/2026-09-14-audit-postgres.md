# Audit PostgreSQL adapter slice

新增 sqlc `InsertAuditEvent` 查询和 `AuditStore.Insert` adapter。写入前复用审计事件必填校验，显式解析 tenant/task UUID，并将 task_id 作为可选租户内引用写入 `public.audit_events`。

验证：sqlc generate、`go test ./internal/data/postgres ./internal/biz/audit`、`go vet ./...`、`go build ./...` 均通过；尚未在真实 PostgreSQL 事务中核验审计回滚语义。
