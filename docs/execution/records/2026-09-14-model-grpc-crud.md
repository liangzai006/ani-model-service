# Model gRPC CRUD handler slice

新增 Model Catalog port 与 PostgreSQL adapter，并实现 gRPC `CreateModel`、`GetModel`、`ListModels`、`DeleteModel`：

- 所有请求先读取可信 Principal，`tenant_id` 仅交叉校验
- Create 校验模型名并生成服务端 UUID
- Get/List/Delete 通过 tenant-scoped Catalog 查询
- Delete 调用软删除，不暴露物理删除
- protobuf capabilities 与 JSONB 之间做边界转换

幂等键持久化、Inference 引用保护和真实 gRPC+PostgreSQL 联调仍待后续切片。

验证：`go test ./internal/service ./internal/data/postgres`、`go vet ./...`、`go build ./...` 均通过。
