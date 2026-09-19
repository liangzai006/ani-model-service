# 2026-09-17 Model 删除引用保护（已验收）

## 已落入实际仓库的代码

Model：

- `api/model/v1/model.proto` 增加 `DeleteModelVersion`；新增 `api/inference/v1/model_reference.proto` 为 Inference 引用协议的客户端副本。Go 输出由固定版本 buf 工作流生成；宿主机容量故障期间使用本地模块缓存代理完成生成，未手改生成代码。
- `internal/data/inference/client.go` 调用版本化引用 RPC，传实际版本 UUID，限制每批 256，5 秒超时；开发凭据绑定配置租户，未知结果返回错误。
- `cmd/ani-model-service/inference_client.go` 和 main 接线。配置 `ANI_INFERENCE_GRPC_ADDR`；TLS 默认启用。只有显式 `ANI_DEVELOPMENT_ENABLED=true`、loopback 地址、有效专用租户和凭据时允许 `ANI_INFERENCE_GRPC_INSECURE=true`。配置缺失继续拒绝删除。
- `internal/data/postgres/deletion_store.go` 在 read committed 事务中锁定模型/版本，再查询远端引用，最后软删除；活动引用返回 MODEL_IN_USE，远端失败回滚；已删除对象重试幂等。移除了 Catalog 的未保护 SoftDeleteModel 入口。对象存储制品保留。
- ready 版本读取对模型和版本加共享锁；创建版本锁父模型并拒绝已删除父模型。必须锁版本本身，否则单版本删除后等待中的查询可能仍返回旧 snapshot。
- 单元、配置、gRPC 客户端测试以及事务删除/并发数据库测试已在实际 Model 仓库。

Inference：

- `api/inference/v1/model_reference.proto` 定义 `CheckModelVersionReferences`。
- `internal/service/model_reference.go` 从可信身份上下文取租户，校验 1–256 个版本 UUID，依赖失败返回 Unavailable。
- `queries/model_reference.sql` / `internal/data/postgres/model_reference.go` 只访问 Inference 自有表：未最终删除服务的所有保留 spec 都算引用，包括 pending、stopped、failed、deleting 及旧 generation。该规则是保守保留，旧版本不会因新 generation 出现就解除保护。
- composition root 注册该服务；显式开发身份允许该只读请求。请求没有可用来建立身份的 tenant 字段。

## 并发保证边界

进一步追踪实际 main 接线发现：Inference 的 Model CreateUseCase 会在持久化之前预查元数据。不能声称所有 Model 读取都发生在 Inference 持久化之后。

真正保障来自 worker `Materializer.EnsureModel`：实例/spec 已持久化后，必须再取 ready ModelVersion；即使下载 Job 已 Complete，也先做这次复查。已有 worker 对应持久引用会被删除检查拦住；删除检查之后才持久化的并发创建，worker 会读到删除并失败，不能变为 ready。创建入口在这个竞态下可能先返回 pending，再由 operation 报失败。本次并未给跨服务 admission 增加分布式预留协议。

拟运行的并发测试包含整模型和单版本两种情况，使用 `pg_blocking_pids` 确认查询真实等待行锁，再放行删除，断言被删除版本不能返回 ready。整模型删除也断言等待中的新增版本不能写入；单版本删除后允许为仍存在的模型创建其他版本。

## 本次验证事实

- A 列表切片：宿主机真实 PostgreSQL 验收和完整 `make verify` 已通过，见列表执行记录。该结果发生在 B 删除修改之前。
- B Inference 服务/数据/启动包测试在宿主机通过；当时未设置 INFERENCE_PG_DSN，所以数据库用例跳过。
- B Model 的 service、postgres 非数据库测试、Inference 客户端测试、配置测试通过。缺少引用检查器的失败测试已观测 RED → GREEN。
- B Model 客户端与 service 的 `go test -race` 通过；全仓库 `go vet` / `go build` 通过。
- sqlc v1.31.1 再生成文件不变，`git diff --check` 通过。
- B 的真实 PostgreSQL 删除、并发、引用 RPC 数据库验收已完成；修改后的完整门禁已通过。

## 验收结果（2026-09-18）

- 已将 `docs/execution/patches/2026-09-17-inference-reference-tests.patch` 应用到实际 Inference 仓库，并执行 gofmt。
- Model：真实 PostgreSQL 的 `TestProtectedDeletionPostgres`、整模型与单版本锁竞争测试、`TestCatalogPaginationPostgres` 全部通过；`make verify` 通过。
- Inference：真实 PostgreSQL `TestModelReferencesGRPCPostgres` 通过；`go test -p 2 ./... -count=1`、`go vet -p 2 ./...`、`go build -p 2 ./...` 全部通过。
- 结果：B 切片完成。现有成功推理部署未替换，未提交或推送。
- 尝试按 requesting-code-review 技能调度只读审查，但工具返回 `agent thread limit reached`；已在主任务中继续审查，未声称独立审查完成。

## 待应用补丁和执行恢复

宿主机审批起初恢复，随后多次返回 `Automatic approval review failed: Selected model is at capacity`。受影响命令均未启动，未通过其他通道绕过拒绝。

Inference 的额外真实数据库 + gRPC 测试、completed Job 的 Model 复查回归断言已写入实际 Inference 仓库。补丁仍保留作审计记录：

`docs/execution/patches/2026-09-17-inference-reference-tests.patch`

补丁已应用；下列命令记录本次执行方式：

```bash
cd /root/kubercon/ani-inference-service
git apply --check /root/kubercon/ani-model-service/docs/execution/patches/2026-09-17-inference-reference-tests.patch
git apply /root/kubercon/ani-model-service/docs/execution/patches/2026-09-17-inference-reference-tests.patch
gofmt -w internal/data/postgres/model_reference_integration_test.go internal/data/postgres/model_reference.go internal/service/model_reference.go internal/data/model/materializer_test.go
```

设置测试数据库连接环境变量时禁止打印凭据。现场安全注入工具 `/tmp/ani-reference-db-test.py` 只在子进程环境中设置 MODEL_DATABASE_URL/INFERENCE_PG_DSN，输出会遮蔽 DSN 和密码。

```bash
cd /root/kubercon/ani-model-service
python3 /tmp/ani-reference-db-test.py go test ./internal/data/postgres -run '^Test(ProtectedDeletionPostgres|DeletionSerializesReadyReadAndVersionCreationPostgres|CatalogPaginationPostgres)$' -count=1 -v
make verify
cd /root/kubercon/ani-inference-service
python3 /tmp/ani-reference-db-test.py go test ./internal/data/postgres -run '^TestModelReferencesGRPCPostgres$' -count=1 -v
go test -p 2 ./... -count=1
go vet -p 2 ./...
go build -p 2 ./...
```

测试仅写随机测试租户。普通数据库测试事务回滚；并发测试需要多连接可见的已提交夹具，会清理同一随机租户自己创建的三张 Model 表数据。

现有成功推理部署保留，两个运行服务未替换，未 commit/push。B 验收后继续完整模型导入/任务、推理更新与原型 API；IAM/Gateway 仍后置。
