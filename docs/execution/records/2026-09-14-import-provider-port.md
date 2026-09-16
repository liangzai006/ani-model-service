# Import provider adapter boundary slice

新增 `internal/data/importer/source.go`：

- 统一 HuggingFace/ModelScope `SourceAdapter` 接口
- 只返回 storage-neutral 的对象引用、格式、大小和 SHA256
- `Registry` 强制 source allowlist
- provider 下载、认证和对象写入由外部 adapter/Storage 负责

验证：`go test ./internal/data/importer`、`go vet ./...`、`go build ./...` 均通过；真实 provider 未配置，标记 `not_verified`。
