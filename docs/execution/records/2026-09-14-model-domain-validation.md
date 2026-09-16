# Model 领域校验切片

已新增 `internal/biz/model`，集中定义模型名称、模型版本、ready 制品完整性和 engine allowlist 校验。`DeploymentInput` 只有在版本 ready 且 provider/reference/SHA256 完整时才通过；`vllm`、`sglang`、`tgi` 是当前允许的 engine 类型，拒绝任意命令引擎。

验证：`gofmt`、`go test ./...`、`go vet ./...`、`go build ./...` 均通过。

当前仍未接入 gRPC handler 和 PostgreSQL use case；本切片只提供可复用领域规则。
