# HTTP Storage adapter slice

**已撤回（2026-09-14）**：以下是历史实现记录，不是已核验的 Storage 契约。
这些 HTTP 路径由本服务单方面假定，没有真实 provider 证据；原源码已移至
`archive/2026-09-14-storage-http.go.txt`，不参与 Go 编译。后续采用外部 Storage
正式契约接入。参见 [Gateway 入口边界纠正](2026-09-14-gateway-boundary.md)。

新增 `internal/data/storage/http.go`，实现 Storage port 到可配置外部 HTTP API 的 adapter：

- `POST /v1/upload-url`
- `POST /v1/object-exists`
- `POST /v1/verify-checksum`
- `POST /v1/download-url`

请求携带 tenant_id/object_ref，provider 非 2xx、网络和 JSON 错误统一归类为 `storage.ErrProvider`。adapter 不创建 bucket/PVC，不管理文件生命周期。

上述 endpoint 仅为当时的实现假设，不能视为正式版本化契约；真实 Storage 服务地址和联调为 `not_verified`。

验证：`go test ./internal/data/storage ./internal/biz/storage`、`go vet ./...`、`go build ./...` 均通过。
