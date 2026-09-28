# GetModelVersion response mapping slice

修正 handler 响应，除完整 `ModelVersion` 外同时返回兼容契约所需的 `Model`（tenant_id、model id）。版本中包含 storage path、checksum 和兼容 engine 字段；Inference 不把这些字段当作默认启动命令。

验证：`go test ./internal/service ./internal/client`、`go vet ./...`、`go build ./...` 均通过。
