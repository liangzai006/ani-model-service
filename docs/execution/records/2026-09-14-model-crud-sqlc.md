# Model CRUD sqlc slice

扩展 `queries/model.sql` 并重新运行 sqlc，新增：

- `ListModels`：显式 `tenant_id`、状态过滤、限制条数
- `SoftDeleteModel`：租户限定软删除
- `CreateModelVersion`：租户与 model 复合外键由 schema 校验
- `ListModelVersions`：显式 tenant/model 条件

`ModelStore` 暴露 Create/Get/List/SoftDelete 模型方法，统一解析 UUID、限制分页上限，并将 0 行软删除映射为 `pgx.ErrNoRows`。

验证：sqlc generate、`go test ./internal/data/postgres`、`go vet ./...`、`go build ./...` 均通过；尚未连接真实 PostgreSQL 执行 CRUD 集成测试。
