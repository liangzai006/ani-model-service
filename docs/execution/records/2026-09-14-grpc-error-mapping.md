# PostgreSQL gRPC error mapping slice

新增统一 `mapStore`：

- `pgx.ErrNoRows` → NotFound
- PostgreSQL `23505` 唯一约束冲突 → Conflict
- 其他存储错误 → Internal/MODEL_STORE_ERROR

已应用于 Model 和 ModelVersion 创建 handler，避免把所有数据库异常误报为 Conflict。

验证：`go test ./internal/service`、`go vet ./...`、`go build ./...` 均通过。
