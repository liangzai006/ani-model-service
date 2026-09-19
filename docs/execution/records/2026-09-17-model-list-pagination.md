# 2026-09-17 Model 列表与版本展示修复

## 后续验收：宿主机已恢复

用户再次授权继续后，宿主机执行恢复。真实 recycling PostgreSQL 上 `TestCatalogPaginationPostgres` 及其全部子用例通过（0.11s，事务回滚）；宿主机 `go test -p 2 ./... -count=1` 和完整 `make verify` 均 exit 0，包含原先被沙箱阻止的 TCP 测试、生成检查、vet/build 和模块校验。下面的容量故障描述保留为历史记录，已不再是当前阻塞。A 列表切片已验收，运行中的服务尚未替换。

## 现场与范围

在实际 `/root/kubercon/ani-model-service` 实现，不是隔离副本。保留之前已有的未提交修改。本批不包含 Inference 改动，未重启服务、修改 Kubernetes 资源、commit 或 push。产品依据为 `/root/design/原型9.8` 的 AI 服务页面。

## 已实现

- ListModels 在 PostgreSQL 中按状态、来源、能力和关键词组合筛选。关键词对 name、display_name、model_id 作不区分大小写的字面子串匹配，百分号不作为通配符。
- 模型及版本列表按 `(created_at,id)` 倒序分页，额外查询一条判断 `has_more`。默认每页 100，最大 1000；负数或超限返回 INVALID_ARGUMENT。
- `next_cursor` 包含版本、列表范围摘要和边界；拒绝损坏、超长、跨租户、跨筛选或跨模型的原游标。游标不是认证令牌，授权始终来自可信 principal，SQL 始终限制 tenant_id。
- 模型映射补齐来源仓库、错误信息、创建/更新时间；版本映射补齐数据库已有的大小、加密元数据和创建时间。ready 版本读取和普通列表保持字段一致。
- Model.versions 返回最新一个未删除版本（没有则为空），包含未 ready 的最新版本；完整历史走 ListModelVersions。对一个模型列表批量查询最新版本，避免逐行查询。
- ModelStore 使用已有 DBTX 接口，可与 VersionStore 在同一只用于测试的事务中验证并回滚；未增加运行时依赖或数据库迁移。

本批只修复已有数据的读取与分页，不代表所有导入/创建路径已完整保存这些元数据，也不代表模型仓库完整功能已完成。

## 本次验证

1. 修复前 `TestListModelsReturnsPageAndContinuation` 失败：请求 2 条实际返回 3 条且无 meta。
2. 修复前 `TestVersionFromRowPreservesMetadata` 失败：已存储字段映射为零值。
3. 修复后：

```bash
GOCACHE=/tmp/ani-model-pagination-go-cache GOPROXY=off \
  go test ./internal/service ./internal/data/postgres -count=1
GOCACHE=/tmp/ani-model-pagination-go-cache GOPROXY=off go vet -p 2 ./...
GOCACHE=/tmp/ani-model-pagination-go-cache GOPROXY=off go build -p 2 -trimpath ./...
```

以上均 exit 0。没有配置 MODEL_DATABASE_URL，数据库集成测试跳过；此结果不能视为真实数据库验收。

4. `sqlc v1.31.1 generate` 再生成前后三个生成文件 SHA256 一致；`git diff --check` 通过。
5. 全量 `go test -p 2 ./... -count=1` 已尝试，沙箱内需要 loopback TCP listener 的 app、importer、runtime 测试因 `socket: operation not permitted` 失败，未宣称全量通过。
6. `make verify` 已尝试，在 GOPROXY=off 环境的 protobuf 生成器版本解析处失败（`loading deprecation ... module lookup disabled`），未完成完整门禁。

## 外部阻塞与续接

宿主机 `go test` 和只读 `docker ps` 的自动审批多次未能启动命令，返回：

```text
Automatic approval review failed:
Error running remote compact task: Selected model is at capacity.
Please try a different model.
```

在核实测试范围、证明沙箱确实禁止 TCP 监听后，重试宿主机测试仍遭相同拒绝。未通过其他通道绕过该拒绝。需要恢复宿主机执行审批通道，再执行以下验证：

- 通过既有安全配置提供 MODEL_DATABASE_URL，禁止输出 DSN/凭据。
- `go test ./internal/data/postgres -run '^TestCatalogPaginationPostgres$' -count=1 -v`。该测试创建新随机租户，在单一事务内写入夹具并始终 Rollback；覆盖同时间排序、页间插入、组合筛选、字面百分号、其他租户相同模型 ID、最新非删除版本、版本游标、字段回填。
- 宿主机 `make verify`，确认所有监听端口测试和生成检查完成。
- 完成 A 验收后继续 B：Inference 版本化引用查询与 Model 删除保护接线，然后版本删除、完整导入/任务、推理升级及 AI 页面 API。

本批代码尚未替换正在运行的 Model 服务。IAM/Gateway 仍后置，不是本次阻塞原因。
