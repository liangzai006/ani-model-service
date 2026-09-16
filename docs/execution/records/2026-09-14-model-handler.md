# Model handler slice

实现 `internal/service/public.go`：`GetModelVersion` 从可信 Principal 获取 tenant，拒绝缺失身份或租户不匹配，并仅返回 ready 且 artifact checksum 完整的版本；错误映射到 Kratos 状态。服务已在 `buildApp` 中注册到 Kratos gRPC server。CRUD 仍保留未实现占位，Model store 未配置时返回 `MODEL_STORE_UNAVAILABLE`。

验证：

- `GOPATH=/tmp/ani-model-gopath GOMODCACHE=/tmp/ani-model-gomodcache GOCACHE=/tmp/ani-model-gocache go test ./cmd/ani-model-service -run '^$'` 通过（编译测试）。
- 同环境 `go vet ./...` 与 `go build ./...` 通过。
- 完整 `go test ./...` 中现有 `cmd/ani-model-service` 生命周期测试因沙箱禁止监听 `127.0.0.1` 失败；其余包通过。该失败是环境限制，非代码断言失败。
