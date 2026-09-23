# Model 服务执行状态

当前范围仅包括 Model 目录、模型导入、制品存储和 Inference 运行时协作；
Notebook、Kubeflow 用户工作流不属于本状态文件。

## 2026-09-23 新集群真实推理验证

- `pass`：通过 Model 正式上传确认接口将固定 revision 的 SmolLM2-135M 归档上传到新集群 MinIO，ModelVersion ready、SHA256/大小校验和 CephFS 物化 Job 均完成。
- `pass`：通过 Inference gRPC 创建 `smollm2-135m-cpu-real`，控制面 operation succeeded，PVC、Deployment、HTTPRoute 均就绪。
- `pass`：通过 Higress NodePort `http://192.168.102.68:30090/v1/completions` 返回非空 vLLM CPU 生成结果；旧集群和旧项目未操作。
- `partial`：新节点直连 Hugging Face 的远程 Import Job 受网络出口限制失败；已记录并使用同一 Model API 的签名上传路径完成真实验证。更新生命周期仍有 `resource_work` stale-generation 重试问题待修复。

## 2026-09-22 新集群验证

- `pass`：新集群 PostgreSQL、MinIO、Model、Inference 控制面和 Higress 已部署；Model 明文 gRPC 可直接按 request tenant 调用。IAM、可信身份、TLS 和配额校验暂不接入，Inference 不使用固定 quota provider。
- `partial`（历史快照）：Inference 物化下载脚本现支持 HTTP/HTTPS，大小限制改为使用 Model 版本的 `size_bytes`；当时真实 Model 导入和推理仍待执行，后续验证见上方 2026-09-23 条目。

## 2026-09-18 continuation

- `pass`：C 已新增 Model `GetImportTask`/`RetryImportTask`，失败任务会重置为 pending、清零尝试次数和进度；HuggingFace/ModelScope provider 支持 manifest 读取，worker 会安全打包完整仓库为 tar 并复用 checksum、artifact、ready 链路。真实 HuggingFace 完整仓库、Storage、artifact、ready、任务查询和重试均已通过，见 `records/2026-09-18-model-manifest-import.md`。
- `partial`：A、B、C 已完成真实数据库/Storage 和完整门禁验收。D 的 ModelVersion 切换已接入 Inference Update：通过 Model gRPC 重新校验 ready 快照并将新版本写入下一代 spec，定向测试、Inference 全量测试/vet/build、Buf lint 和真实 PostgreSQL 更新测试通过；通用引擎/容量、真实生命周期恢复仍待完成。E 的调用测试、策略和 429 仍未实现。

以上为 2026-09-18 的历史阶段记录；当前状态以本文件顶部的 2026-09-23 条目为准。

## 2026-09-20 continuation

- `partial`：导入入口已改为独立 Kubernetes Job 路径。Model worker 创建 Job/PVC 并观察状态，`cmd/minio-provision` 在 Job 中先幂等检查/创建共享或租户 bucket，再用 ModelScope/Hugging Face CLI 下载并流式上传 tar；结果经对象存在性和 SHA-256 校验后写 artifact/ready。StorageClass 可使用集群默认值，容量可按 provider manifest 动态估算；该阶段尚未验收真实集群 Job、大模型及 Inference 物化；后续新集群远程 Job 已执行但受 Hugging Face 出口限制，签名上传后的 Inference 物化与 CPU 推理已于 2026-09-23 验收。详见 [Kubernetes import Job](records/2026-09-20-kubernetes-import-job.md)。

## 当前状态

- `pass`：固定 `ani-kratos-layout` 骨架生成、Kratos gRPC/admin/health 生命周期、API 配置 Proto 迁移到 `api/model/v1`。
- `pass`：兼容 Model gRPC Proto、生成 client/server 和运行时默认 engine 字段。
- `pass`：Model `public` schema migration、sqlc 查询生成，并已在本机 PostgreSQL 的 `recycling` 数据库真实应用和只读核验。
- `decision`：新集群 Model 与 Inference 使用同一 PostgreSQL 实例、分别使用 `ani_model` 与 `ani_inference` 数据库，目前共用访问账号；账号隔离列为后续部署加固项。
- `decision`：导入任务采用 PostgreSQL 持久任务表 + worker，暂不采用 CRD/controller-runtime；CRD 未来只能作为独立入口适配器，不能替代 Model 的核心业务存储。
- `pass`：模型名称、版本 ready 制品完整性和 engine allowlist 领域校验及测试。
- `pass`：`public.model_artifacts` sqlc 创建/查询与 tenant 限定的 PostgreSQL adapter 已实现，并用本机 Docker PostgreSQL 验证跨租户读取拒绝；制品内容和 Storage 生命周期仍由外部 Storage 负责。
- `pass`：ModelVersion ready/error CAS 状态更新已实现；ready 仅接受同租户、非空引用且 SHA256 与版本摘要匹配的制品，并已用本机 Docker PostgreSQL 验证。
- `partial`：上传确认路径已接入 `CreateModelVersion`：Storage 对象存在性和 SHA256 校验通过后写入 `model_artifacts`，再执行 ready CAS；成功重放直接返回 ready 版本。2026-09-23 已通过正式 GetUploadURL → MinIO 签名 PUT → CreateModelVersion 跑通真实上传确认；版本 ready、SHA256 和大小均已核验。
- `partial`：DeleteModel 已接入版本化 Inference 引用检查端口；有引用或检查未知时拒绝软删除，正式 Inference gRPC/API checker 仍未配置。
- `pass`：租户字段校验、资源作用域过滤和 Model ↔ Inference 的版本快照协作已实现；Inference 真实仓库的全量测试、vet、build、Buf lint 和 ModelVersion PostgreSQL 更新测试通过。2026-09-23 新集群双进程部署、模型物化、推理创建及实际调用已通过；更新生命周期仍存在 stale-generation 重试问题。详见 [Model–Inference contract](records/2026-09-16-model-inference-contract.md) 和 [ModelVersion switch](records/2026-09-18-model-version-switch.md)。
- `pass`：开发机网络环境下 `make verify` 全部通过，包含 Buf 1.60.0 固定版本校验、生成稳定性、`go test ./...`、`go vet ./...`、`go build ./...`、`go mod verify` 和 `git diff --check`。
- `pass`：宿主机 `govulncheck` 已发现并修复 grpc、pgx 和 x/crypto 的直接调用链漏洞；升级至 `google.golang.org/grpc v1.83.2`、`github.com/jackc/pgx/v5 v5.9.2`、`golang.org/x/crypto v0.56.0` 后再次扫描显示 0 个代码受影响漏洞。Gitleaks 扫描无泄漏。
- `pass`：在不修改当前仓库历史的隔离 clean 快照中完成 CycloneDX SBOM、license review 和 supply-chain 验证；当前仓库已同步 `docs/scaffold/bom.cdx.json` 与 `docs/scaffold/license-review.md`。SBOM 包含 52 个运行时组件，license evidence 完整，固定 layout notice hash 校验通过。
- `pass`：导入 worker、Provider 下载和 Storage 上传现在输出带 tenant/task/provider 阶段字段的 JSON 结构化日志；本地终端和 Kubernetes `kubectl logs -f` 查看方式已记录。真实 HuggingFace→MinIO E2E 已捕获完整生命周期日志并以退出码 0 完成，见 `docs/execution/records/2026-09-15-import-e2e.md`。
- `pass`：`ImportModel` 成功落库后会非阻塞唤醒同一 Model Pod 内的持久 worker；真实 E2E 已捕获 `import worker wake requested`，随后完成租户 bucket 检查/创建、Provider 下载、MinIO 上传、checksum、ready 和 completed。worker 仍保留 PostgreSQL due scan 作为重启兜底。见 `docs/execution/records/2026-09-16-tenant-bucket-import-job.md`。
- `pass`：租户 bucket 首次真实创建、已有 bucket 的再次真实导入及 ready 版本重放均已通过。新增可重复集成测试验证同幂等键返回同一任务、下载 SHA256 一致、短期地址过期返回 HTTP 403；正式 Kubernetes crash/restart 仍为 `not_verified`。见 [Existing bucket replay](records/2026-09-16-existing-bucket-replay.md)。
- `partial`：Model gRPC `GetModelVersion` handler 已实现 request tenant 过滤、ready 制品完整性检查并注册到 Kratos server；sqlc CRUD 查询、ModelStore 基础 CRUD 与 gRPC Model CRUD handler 已补齐；导入任务、CreateModel 和 CreateModelVersion 均已持久化 request fingerprint，支持同 payload 重放、不同 payload 冲突；跨资源审计事务仍待接入；ModelVersion gRPC 创建/列表 handler 与 VersionCatalog PostgreSQL adapter 已补齐。
- `partial`：已新增版本化 `api/storage/v1` gRPC 契约和 `GRPCAdapter`，并由组合根通过 `ANI_STORAGE_GRPC_ADDR` 注入 Model；当前新环境使用直接 MinIO 存储路径。bufconn 成功/失败语义、runtime 和 `make verify` 已通过；正式 Storage 服务端接入、真实对象 checksum 和地址过期验证仍为 `not_verified`。自行假定的 HTTP adapter 已撤出编译源码并归档。
- `pass`（边界核对）：明确客户端 HTTP → 独立 Higress Gateway → vLLM HTTP；Model 仅保留运维 HTTP，不实现 Gateway。新集群已移除 APISIX，Higress 通过 Gateway API 和 model-router 按请求体 `model` 设置 Header，再以固定 `/v1/completions` 路径和模型 Header 精确分流；Model 业务 HTTP → gRPC 转换入口仍未接入，当前直接使用 Model gRPC。见 [入口边界纠正](records/2026-09-14-gateway-boundary.md)。
- `partial`：导入任务 lease/CAS fencing 领域状态机及 PostgreSQL sqlc adapter 已实现并测试；worker due scan、lease renew、执行循环已实现；ImportModel 现在把 source/repo_id/revision/idempotency_key 持久化到任务，worker 可据此选择外部 provider；已实现 provider→Storage 对象存在性/checksum 执行器；远程任务支持先不绑定 model/version，避免从 repo_id 伪造模型身份；provider 元数据解析、按租户查找既有模型/版本和 lease CAS 绑定已通过真实 HuggingFace→PostgreSQL/MinIO E2E；新增真实 PostgreSQL 过期 lease 重领测试覆盖“进程中断后重启 worker”核心 fencing 语义，正式 Kubernetes crash/restart 仍未完成。
- `partial`：worker 对已绑定版本的成功任务会先执行 ready CAS，再确认任务 completed；ready 失败进入 retry。未绑定远程任务现在先执行 provider 元数据绑定，失败时保持 pending 并重试。
- `pass`：导入 worker 增加有界重试和 `failed` 终态；默认五次尝试，provider、Storage、checksum、绑定和 ready finalization 错误在达到上限后通过 owner/lease_epoch CAS 写入失败状态，旧 worker 结果仍被 fencing。
- `pass`：远程导入在 provider 未提供摘要时，会在 Storage 内容校验后以一次性 CAS 写入版本 checksum/大小，再执行 artifact 和 ready；预置且一致的摘要可幂等重放，不一致摘要拒绝。
- `partial`：审计事件领域结构与必填字段校验已实现；Model、ModelVersion、ImportModel 成功受理路径已接入 `audit.Store` 和 PostgreSQL adapter；worker backlog/retry/checksum/provider 指标已接入 Prometheus registry。跨资源原子事务仍待接入。
- `partial`：Readiness 已支持 PostgreSQL/Storage/worker 依赖门控；新增 WorkerSupervisor 管理 durable worker 启停，worker 循环退出会清除 readiness。组合根现已注入 PostgreSQL WorkStore、正式 provider 和 worker 实例；进程重启/crash 集成测试仍未完成。
- `pass`：已实现可配置 HuggingFace/ModelScope HTTP source adapter、完整 manifest tar、流式上传到 Storage、对象存在性和 SHA256 校验，并用真实 HuggingFace 完整仓库和集群 MinIO 跑通 gRPC→worker→artifact→ready→下载及任务重试；正式 Kubernetes crash/restart 仍 `not_verified`。
- `pass`：修正远程导入任务 schema 与运行语义不一致的问题；`model_import_tasks.model_id` 允许在 provider 元数据解析前为空，并提供兼容已有数据库的 `000002` migration；ModelVersion 的 `model_id` 仍必填。见 [unbound import task schema](records/2026-09-15-unbound-import-task-schema.md)。
- `pass`：`ImportModelRequest` 现支持可选 Model/ModelVersion 绑定；绑定任务执行成功后会持久化 artifact 并通过 lease/CAS 置版本 ready，artifact 重试具备不可变事实校验；未绑定任务会由 provider 元数据解析并绑定到既有模型版本。真实 provider、PostgreSQL、Storage 和完整 manifest 绑定联调已通过。见 [provider metadata binding](records/2026-09-15-provider-metadata-binding.md) 和 [完整仓库导入](records/2026-09-18-model-manifest-import.md)。
- `pass`：worker 已阻止未绑定任务被错误完成；此类任务保持 pending 并通过 CAS retry 延后，只有绑定 `model_version_id` 的任务才会执行 provider、写 artifact 和置 ready。见 [unbound worker deferral](records/2026-09-15-unbound-worker-deferral.md)。
- `pass`：worker 续租 CAS 失败会立即取消 provider/Storage 执行并进入 retry，旧 lease 不能继续完成任务。见 [lease renewal fencing](records/2026-09-15-lease-renew-fencing.md)。
- `pass`：正式 Model client 的 ready 版本映射已修正；空启动命令不再生成伪造的 `command_argv[0]`，并保留 artifact 引用、SHA256 和 engine 默认值映射。见 [Inference runtime mapping](records/2026-09-15-inference-runtime-mapping.md)。
- `partial`：已只读发现集群 MinIO `ani-s05-objectstore/ani-s05-minio`，S3 NodePort `10.10.1.68:30900` 的 readiness 检查成功；凭据来自 `ani-s05-minio-root` Secret 引用但未读取值。未发现独立 Storage gRPC Service，正式跨服务 Storage 契约联调仍为 `not_verified`。见 [MinIO discovery](records/2026-09-15-minio-discovery.md)。
- `pass`：新增 MinIO S3 Storage adapter，支持共享 bucket 检查和租户 UUID bucket 的按需检查/创建、流式上传、对象存在性、真实内容 SHA256 和短期下载地址；已完成真实租户 bucket 导入、对象读写/checksum 和 1 秒预签名地址过期验证。见 [MinIO S3 adapter](records/2026-09-15-minio-adapter.md) 和 [Import E2E](records/2026-09-15-import-e2e.md)。
- `pass`：共享 bucket 兼容配置默认值为 `ani-models`；启用 `ANI_MINIO_TENANT_BUCKETS=true` 后按租户 UUID 检查并幂等创建 bucket，账号和密钥仍通过 Kubernetes Secret 环境注入。见 [MinIO configuration defaults](records/2026-09-15-minio-config-defaults.md)。
- `pass`：已通过独立 `cmd/minio-provision` 在集群 MinIO 中创建 `ani-models` bucket；租户 bucket 也可通过 `ANI_MINIO_TENANT_ID` 显式初始化，运行时缺失时由导入 job 幂等创建。凭据来自现有 Secret 的临时环境注入，未写入仓库，也未修改 PVC/Deployment。见 [MinIO provision command](records/2026-09-15-minio-provision-command.md)。
- `pass`：补充 `docs/deploy/minio-env.yaml`，固定集群 MinIO Service、默认 bucket 和 Secret key 映射；仅为注入片段，不包含 Secret 值或远程 apply。
- `pass`：新增 `public.models.model_id` 作为租户范围唯一的外部模型标识，历史数据由 `name` 回填；内部 UUID `models.id` 与版本外键保持不变。`GetModelVersion` 支持兼容 UUID 查询及 `model_id + version` ready 查询，响应同时返回外部模型标识；跨租户始终按 request tenant 限定。已在本机 PostgreSQL 回滚事务中验证外部 ready 查询和跨租户结果；持久 migration 尚未重新应用。见 [External Model ID](records/2026-09-15-external-model-id.md) 和 [PostgreSQL transaction check](records/2026-09-15-external-model-id-postgres-transaction.md)。
- `pass`：模型 CRUD、创建版本和版本列表统一解析外部 `model_id` 到租户内的模型 UUID；UUID 兼容路径保留，删除模型会同时检查 Inference 可能持有的 UUID 与外部标识引用。真实 PostgreSQL 外部 ID 查询已在端到端导入中验证。见 [External model ID CRUD](records/2026-09-15-external-model-id-crud.md)。

证据：[layout baseline](records/2026-09-14-layout-baseline.md)、[Model gRPC contract](records/2026-09-14-model-grpc-contract.md)、[Model PostgreSQL schema](records/2026-09-14-model-postgres-schema.md)、[domain validation](records/2026-09-14-model-domain-validation.md)、[worker supervisor](records/2026-09-15-worker-supervisor.md)、[PostgreSQL worker decision](records/2026-09-15-postgres-worker-decision.md)、[WorkStore runtime injection](records/2026-09-15-workstore-runtime-injection.md)、[provider streaming worker](records/2026-09-15-provider-streaming-worker.md)。

## 2026-09-23 大模型真实验证

- `pass`：不是只验证 279M 小模型；已在新集群通过正常 Model 上传确认链路验证固定 revision 的 Qwen2.5-1.5B，归档 `3,098,992,640` 字节，SHA256 为 `d0f3c1294ac34d9ca84858cf93bfe4dbae8ebdfa2a317d1f90970b1dd85f11dd`，ModelVersion `515adc62-8694-490e-9e23-a6e7d436c984` ready，`sizeBytes` 已持久化。
- `pass`：6Gi CephFS PVC、物化 Job、vLLM CPU 运行时和 Inference Create operation 全部完成；通过新集群 Higress NodePort 的真实 `/v1/completions` 返回 HTTP 200 和非空文本。
- `limit`：新集群无 GPU，节点约 8 CPU/15Gi 可分配内存；单个 1.5B CPU 模型已接近容量边界，3B/7B 需要更大内存或 GPU 节点。大模型验证应串行执行。
- `pass`：服务名含点号会在生成 endpoint Service 时触发 DNS-1035 限制，已用 `qwen-1-5b-cpu-sizefix` 完成验证；多个模型共用固定 `/v1/completions`，由 Higress model-router 从 JSON `model` 设置 Header，再由 HTTPRoute 精确匹配，已消除同路径路由冲突。

- `cleanup`：点号命名的失败测试 Deployment 已在新集群缩容到 0，避免继续占用 1.5B CPU/内存；其 active create operation 因当前 API 没有 cancel 且会阻塞 Stop/Delete，仍需后续增加可取消操作或做受控状态修复。旧环境未操作。
