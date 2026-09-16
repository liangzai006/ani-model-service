# Public schema migration correction

按用户最终要求，Model 的 PostgreSQL 表改为 `public` schema：

- `public.models`
- `public.model_versions`
- `public.model_artifacts`
- `public.model_import_tasks`
- `public.audit_events`

已同步修改 `migrations/000001_model_control_plane.sql`、`queries/model.sql`、sqlc 生成代码引用和 worker 集成测试。sqlc 生成后的 Go 类型采用 `Model`、`ModelVersion`、`ModelImportTask` 等名称，相关 adapter 已修正。

本机 `recycling-postgres/recycling` 数据库已应用 public migration，并重新通过真实 `TestWorkStoreLeaseRecoveryPostgres`。此前误建的 `model` schema 未删除，以避免不可逆清理；Model 运行代码不再引用它，后续可由数据库管理员按变更窗口清理。

验证：`sqlc generate`、`go test ./internal/data/postgres -run TestWorkStoreLeaseRecoveryPostgres -count=1`、`go vet ./...`、`go build ./...` 均通过。
