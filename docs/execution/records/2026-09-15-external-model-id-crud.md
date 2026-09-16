# External model ID CRUD resolution

日期：2026-09-15

`GetModel`、`DeleteModel`、`CreateModelVersion` 和 `ListModelVersions` 现在接受
稳定外部 `model_id`。handler 在可信 Principal 的 tenant 范围内解析
`public.models.model_id`，随后只把内部 UUID 传给版本表和软删除查询；UUID
输入仍按兼容路径处理。

删除前的 Inference 引用检查同时使用内部 UUID 和外部标识，任一标识存在活动引用
都会拒绝删除；检查异常继续 fail closed。版本列表和创建响应返回外部模型标识。

验证：`go test ./internal/service ./internal/data/postgres -count=1`、
`go vet ./...`、`go build -trimpath ./...` 和 `buf lint` 通过；新增测试覆盖外部/UUID
解析、跨租户拒绝、大小写敏感的不存在模型、删除双标识引用检查及 provider 错误。
隔离 PostgreSQL 中应用 `000006_external_model_id.sql` 后的真实 CRUD 查询尚未执行，
标记为 `not_verified`。
