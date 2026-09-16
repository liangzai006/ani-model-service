# Inference runtime mapping

日期：2026-09-15

## 实现

- Model client 继续只通过 `GetModelVersion` 读取 ready 版本，不访问 Model 数据库。
- `storage_path` 映射为 `ArtifactRef`，`checksum_sha256` 映射为 `ArtifactSHA256`。
- `engine_type` 映射为 `EngineRuntime`。
- 仅在 `startup_command` 非空时把它放入 `command_argv` 首项；无命令时保持空 argv，不再产生空字符串命令。

## 验证

- 新增空命令和 command+args 映射测试。
- `go test ./internal/client ./internal/worker ./internal/service` 通过。

## 未验证

- Inference 正式进程与 Model gRPC 的双进程联调仍为 `not_verified`。
