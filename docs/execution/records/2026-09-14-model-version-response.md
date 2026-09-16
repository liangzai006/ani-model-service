# GetModelVersion response mapping slice

修正 handler 响应，除完整 `ModelVersion` 外同时返回兼容契约所需的 `Model`（tenant_id、model id）。版本中包含 storage path、checksum、engine 默认字段，供旧客户端和 Inference client 使用。

验证：`go test ./internal/service ./internal/client`、`go vet ./...`、`go build ./...` 均通过。
