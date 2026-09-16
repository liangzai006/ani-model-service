# ModelVersion gRPC handler slice

新增 `VersionCatalog` port，并实现 `CreateModelVersion` 与 `ListModelVersions` handler：

- 从可信 Principal 获取 tenant，拒绝缺失身份或交叉租户请求
- 创建时生成服务端版本 UUID，校验 format、版本必填字段和 engine allowlist
- 列表查询使用 tenant + model 条件与受限 limit
- protobuf 运行时默认字段映射到领域 Version

真实 VersionCatalog/PostgreSQL 写入、幂等键和 artifact 完整性仍待后续切片。

验证：`go test ./internal/service ./internal/biz/model`、`go vet ./...`、`go build ./...` 均通过。
