# Storage adapter boundary slice

新增 `internal/biz/storage` 外部端口，覆盖上传地址、对象存在性、checksum 校验和短期下载地址。端口只描述外部 Storage API，不创建 bucket、PVC 或管理文件生命周期。

同时提供通用 SHA256 reader 校验和签名 URL 过期校验，单元测试覆盖正确摘要、摘要不匹配及过期 URL。

验证：`go test ./internal/biz/storage ./internal/service`、`go vet ./...`、`go build ./...` 均通过；尚未接入真实 Storage provider，标记 `not_verified`。
