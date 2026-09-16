# Shared PostgreSQL runtime wiring

按用户确认，Model 运行时沿用同级 Inference 的 `ANI_DATABASE_DSN` 环境变量，暂时使用同一 PostgreSQL 数据库和访问账号。

- `cmd/ani-model-service/main.go` 使用 `pgxpool.New` + `Ping` 创建连接池
- 注入 `ModelStore`、`VersionStore`、`CatalogStore` 到 Model gRPC service
- 未设置 DSN 时保持 store unavailable，不伪造数据库依赖
- Model 查询已统一使用 `public` 中的 Model 表；不访问 Inference 表
- pool 在进程退出时关闭

验证：`go test ./cmd/ani-model-service -run '^$'`、`go vet ./...`、`go build ./...` 均通过。完整生命周期测试仍受沙箱禁止监听 `127.0.0.1` 影响。
