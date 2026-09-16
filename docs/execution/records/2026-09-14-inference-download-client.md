# Inference download client slice

扩展正式 Model client，新增 `GetArtifactDownloadURL`：调用 `GetModelDownloadURL` 获取短期地址，校验 URL 和 expires_at，返回 Inference 使用的下载快照。不访问 Model 数据库。

验证：`go test ./internal/client`、`go vet ./...`、`go build ./...` 均通过；真实 gRPC 地址过期联调尚未执行。
