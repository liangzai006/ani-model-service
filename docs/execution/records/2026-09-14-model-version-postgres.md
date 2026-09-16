# ModelVersion PostgreSQL adapter slice

新增 `internal/data/postgres/version_store.go`，实现 VersionCatalog：

- CreateVersion 写入 `public.model_versions`
- GetVersion 仅通过 tenant-scoped ready 查询返回部署输入
- ListVersions 通过 tenant + model 显式条件查询
- startup_args 在 JSONB 与 Go slice 间转换
- UUID 和 limit 在 adapter 边界校验

验证：`go test ./internal/data/postgres ./internal/service`、`go vet ./...`、`go build ./...` 均通过；尚未执行真实数据库集成测试。
