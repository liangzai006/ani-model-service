# Download URL handler slice

实现 `GetModelDownloadURL`：

- 可信 Principal 决定 tenant，tenant_id 仅交叉校验
- 通过 VersionReader 读取并校验 ready + artifact 完整性
- requester 为空时使用可信 actor，不能成为身份凭证
- 通过外部 Storage port 请求 5 分钟短期下载地址
- 对 provider 错误、空地址和已过期地址返回明确错误

尚未接入真实 Storage provider，仍需完成上传 URL 与对象存在性流程。

验证：`go test ./internal/service`、`go vet ./...`、`go build ./...` 均通过。
