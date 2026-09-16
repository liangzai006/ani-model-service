# Provider metadata binding

日期：2026-09-15

未绑定远程任务在下载前通过 provider `MetadataSource` 解析外部模型标识和版本：
仓库最后一段作为外部 `model_id`（如 `Qwen/Qwen3-32B` → `Qwen3-32B`），revision
作为版本标识，缺省为 `main`。PostgreSQL adapter 按任务 tenant 查找既有模型及
`pending/importing/error` 版本。

worker 使用当前 lease owner/epoch 执行 `BindImportTask` CAS。绑定成功后才执行
provider 下载、Storage 上传、对象存在性/SHA256 校验、artifact 持久化和 ready CAS；
元数据缺失、模型/版本不存在或 lease 失效都会重试，不会从 repo 名称创建资源。

`ImportModel` 的可选 `model_id` 同时接受内部 UUID 和外部标识；外部标识在受理时
按可信 tenant 解析为内部 UUID 后落库。

验证：provider metadata、worker binding 和整仓 `go test ./...` 通过；`go vet ./...`、
`go build -trimpath ./...`、`buf lint` 通过。真实 provider、PostgreSQL BindImportTask
和进程重启恢复仍为 `not_verified`。
