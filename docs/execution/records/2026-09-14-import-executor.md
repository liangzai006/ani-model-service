# Import executor provider/storage boundary

新增 `internal/worker/import_executor.go`，将导入执行过程连接到已定义的外部端口：

1. 通过 SourceAdapter registry 解析 HuggingFace/ModelScope provider。
2. Fetch 获取 storage-neutral 的对象引用、格式、大小和 SHA256。
3. 调用 Storage `ObjectExists` 检查对象存在。
4. 调用 Storage `VerifyChecksum` 校验摘要。
5. provider、对象缺失、checksum 错误分别返回可分类错误，供 worker Retry。

执行器不假定 Storage 是 HTTP 还是 gRPC，也不管理 bucket/PVC/文件系统。

验证：`go test ./internal/worker`、`go vet ./...`、`go build ./...` 均通过；真实 provider/Storage 仍待正式契约和凭据。
