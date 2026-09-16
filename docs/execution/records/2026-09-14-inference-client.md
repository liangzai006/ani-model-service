# Inference Model client slice

新增 `internal/client/inference.go`：

- 仅依赖版本化 Model gRPC client，不访问 Model 数据库
- `EnsureModel` 调用 `GetModelVersion`
- 拒绝非 ready、缺少 artifact ref 或 checksum 的版本
- 映射 `storage_path → ArtifactRef`
- 映射 `checksum_sha256 → ArtifactSHA256`
- 映射 engine 默认配置到 `EngineRuntime/CommandArgv`

尚未执行双进程真实 gRPC 联调，错误重试策略由 Inference 上层决定。

验证：`go test ./internal/client ./internal/service`、`go vet ./...`、`go build ./...` 均通过。
