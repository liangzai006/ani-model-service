# External model ID API lookup

日期：2026-09-15

## 切片

为 `GetModelVersion` 增加调用方使用的稳定模型标识查询：

```text
tenant_id + model_id (例如 Qwen3-32B) + version (例如 v2)
```

内部 `models.id` UUID 和 `model_versions.model_id` 外键保持不变。旧的
`model_version_id` UUID 查询继续支持兼容调用；请求不能混用两种 selector，且
`model_id` 与 `version` 必须成对出现。

## 实现

- `Model.model_id` 暴露稳定外部标识，`CreateModelRequest.model_id` 可显式提供；
  未提供时新模型以 `name` 作为默认外部标识。
- sqlc 新增 `GetReadyModelVersionByExternalRef`，通过同租户 `models` join、
  `model_id`、版本号、非删除模型、ready 状态和完整 SHA256 制品共同限定。
- `VersionReferenceReader` 作为独立领域端口，PostgreSQL adapter 返回内部 UUID
  和外部模型标识；handler 先校验 Principal tenant，再执行 selector 解析。
- gRPC 响应的 `Model.id` 保持内部 UUID，`Model.model_id` 和
  `ModelVersion.model_id` 返回外部标识，Inference client 可直接使用该标识。

## 验证

- `buf lint`：通过。
- `go test ./internal/data/postgres ./internal/service`：通过。
- `go test ./internal/data/importer`（开发机网络权限）：通过。
- `go vet ./...`、`go build -trimpath ./...`：通过。
- 新增服务 selector、混合 selector 拒绝测试：通过。
- `000006_external_model_id.sql` 已应用到本机 `recycling` 开发数据库；端到端导入
  进一步验证了真实 PostgreSQL 外部标识查询。
