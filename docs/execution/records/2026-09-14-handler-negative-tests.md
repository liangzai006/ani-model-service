# Handler negative-path verification slice

为 Model gRPC handler 增加隔离 fake 验证：

- Principal tenant 与请求 tenant 不一致时返回 PermissionDenied
- 上传 checksum 非 64 位十六进制时返回 InvalidArgument
- 可信上下文缺失时不调用底层 reader

验证：`go test ./internal/service`、`go vet ./...`、`go build ./...` 均通过。
