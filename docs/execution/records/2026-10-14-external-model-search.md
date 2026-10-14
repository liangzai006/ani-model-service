# 外部模型搜索与导入验证记录（2026-10-14）

## 已实现内容

Model 服务增加了统一的外部模型搜索链路：

1. `SearchExternalModels` 接收 provider、关键词、筛选条件和分页游标。
2. provider 仅允许 ModelScope 和 Hugging Face；ModelScope 默认 revision 为
   `master`，Hugging Face 默认 revision 为 `main`。
3. 搜索结果只返回候选信息，不写入本地 Model/ModelVersion，不创建 ImportTask，
   也不会启动下载。
4. 用户确认候选后，按 `CreateModel` → `CreateModelVersion` → `ImportModel` 提交。
   ImportModel 绑定确认后的 `source`、`repo_id`、`revision` 和本地
   `model_id`、`model_version_id`，导入整个远程仓库。
5. Governance 的本地工作树增加了 `POST /api/v1/models:search-external` 转发，
   只复制请求/响应字段，不包含 ModelScope 或 Hugging Face 的 provider 逻辑。
   Governance 提交为 `6edc096`，只保存在隔离工作树
   `/tmp/ani-governance-model-search-20261014`，未推送、未合并。

搜索结果不包含镜像或启动命令。引擎类型、镜像和完整启动命令在创建
ModelVersion 时由调用方提交；服务不会解析、拆分或拼接启动命令。

## Model 仓库提交

本次链路对应的 Model 提交为：

- `096ca15`：定义外部搜索业务端口。
- `0e94609`：实现 ModelScope/Hugging Face provider 搜索适配器。
- `8662c36`：修正 provider 搜索分页与筛选条件。
- `b50d161`：暴露 Model gRPC 搜索接口并接入运行时依赖。
- `057da10`：修正外部搜索授权策略。

本记录提交本身只修改文档，不包含 Governance 代码。

## 已执行检查

已执行并通过：

- `GOWORK=off go test ./internal/data/importer -run 'Test.*Search' -count=1`
- `GOWORK=off go test ./internal/data/importer -count=1`
- `GOWORK=off go test ./internal/service -run 'TestSearchExternalModels' -count=1`
- `GOWORK=off go test ./internal/service ./internal/data/importer -run 'Test(SearchExternalModels|.*Search)' -count=1`
- `GOWORK=off go test ./... -count=1`
- `GOWORK=off make config`
- `git diff --check`

Governance 已执行 provider 转发、租户上下文和服务映射的定向测试，以及 API 生成和
`git diff --check`。Governance 全量 service 测试没有宣称通过：已有的
`TestTaskService_RealAsynqSchedulerLifecycle` 需要配置 `ANI_TEST_REDIS_URI` 才能
运行 Redis 生命周期测试。本次没有进行真实外网 ModelScope/Hugging Face 搜索，也
没有进行真实数据库或集群导入验证，因此本记录不把这些检查描述为真实环境通过。
