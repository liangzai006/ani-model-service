# Model gRPC 契约切片

## 完成内容

- 新增 `api/model/v1/public.proto` 和生成的 Go message/gRPC client/server。
- 保留旧原型的十个方法名称和主要字段语义。
- `ImportTask` 保留旧 `task_id/task_type/status/location_url` 字段编号，并增量增加模型引用和进度字段。
- `GetUploadURLResponse.doc_id` 保留旧字段编号，增量增加 `model_version_id`。
- `ModelVersion` 增量增加 `status`、`engine_type`、`startup_command`、`startup_args`。
- 配置 Proto 继续位于 `api/model/v1`，不回到 `internal/conf`。

## 验证

- `/tmp/model-tools/buf lint`：pass
- `/tmp/model-tools/buf generate`：pass
- `go test ./...`：pass
- `go vet ./...`：pass
- `go build ./...`：pass

当前仅完成契约和生成代码；服务实现、身份校验、数据库和真实跨服务调用仍未完成。
