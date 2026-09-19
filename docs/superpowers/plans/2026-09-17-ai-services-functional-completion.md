# AI 服务基础功能修复计划

用户已认可 2026-09-17 原型核对记录及修复顺序。本计划在当前两个实际仓库顺序执行，保留未提交改动与成功部署，不 commit/push。

## 范围与设计

唯一产品来源是 `/root/design/原型9.8` 的 AI 服务三个页面。Model 保持元数据、制品与导入任务所有权，Inference 保持模型实例、operation 与运行资源所有权；服务间只调用版本化 gRPC，不跨库查询。IAM/Gateway 后置。

首个实现切片是列表与删除：复用既有 ListModels/ListModelVersions 契约，使用 `(created_at,id)` 倒序游标，游标绑定租户、过滤条件及模型范围；SQL 始终以可信租户过滤。筛选在数据库内执行，分页多取一条判断 has_more。时间、大小、来源等字段从数据库回填。删除通过 Inference 查询指定版本的活动引用，未知结果拒绝删除，不根据开发模式跳过检查。

不采用客户端拉取全量再过滤，也不把独立服务数据表拼接查询。暂不扩展明确标记“规划”的评测、优化和协作审批。

## 执行清单

- [x] A：模型列表关键字/来源/能力筛选、稳定游标分页、版本分页与展示字段。
  - [x] 分页截断失败测试复现；跨租户/跨筛选游标、版本范围、空列表和字段映射测试通过。
  - [x] 增加领域查询参数，修改 sqlc 查询并生成，handler 编解码游标和分页 meta；最新版本批量加载。
  - [x] 宿主机执行已恢复；真实 PostgreSQL 事务回滚式验收、全量测试及 make verify 均通过。
- [x] B：Model 删除引用检查客户端与 Inference 版本引用查询接线；版本删除保护。
  - [x] 两个实际仓库已实现版本化 gRPC、客户端、composition root 接线、单版本删除及行锁保护。
  - [x] 单元测试验证身份/参数、活动引用和依赖失败；Model 客户端和 service 竞态检测通过。
  - [x] 真实数据库删除、重试与并发验收，Inference 引用 gRPC 数据库测试。
  - [x] 应用 Inference 测试补丁并运行最终完整门禁。详见 `docs/execution/records/2026-09-17-model-deletion-references.md`。
- [x] C：完整模型仓库导入、制品组织与任务查询/重试。
  - [x] 增加 GetImportTask/RetryImportTask gRPC、进度/错误/时间映射，失败重试清零尝试次数并通过 tenant/CAS 查询。
  - [x] provider manifest 读取和安全 tar 组织，复用 Storage checksum、artifact 和 ready finalization；单元、PostgreSQL 任务测试及 Model `make verify` 通过。
  - [x] 用真实完整 provider repository 跑通 manifest → Storage → artifact → ready，并记录端到端任务查询/重试结果。详见 `docs/execution/records/2026-09-18-model-manifest-import.md`。
- [ ] D：推理切换 ModelVersion、解除样例规模限制，真实生命周期与恢复验收。
  - [x] Update 请求支持 `model_version_id`；Inference 通过 Model gRPC 重新校验 ready 快照，下一代 spec 持久化新版本；定向测试、Inference 全量测试/vet/build、Buf lint 和真实 PostgreSQL 更新测试通过。
  - [ ] 解除通用引擎、PVC 容量和 runtime 规模限制，并验证非 vLLM/分布式路径。
  - [ ] 在专用 Kubernetes 资源上完成 create → ready → stop → start/restart → update version → delete，以及 Pod/worker 重启恢复。
- [ ] E：调用测试、日志/指标/事件、限流策略与真实 429 验证。

每个切片先验证失败再实现，完成后更新本清单与执行记录。Model 执行 `make verify`；Inference 执行相应测试及生成校验。网络、数据库、Kubernetes 验证在宿主机运行，仅使用专用测试资源。后续切片在执行前依据实际代码补充具体设计，不把计划项当成已实现。

2026-09-18 当前验证与续接入口见 [完整仓库导入记录](../../execution/records/2026-09-18-model-manifest-import.md)。A、B、C 已验收，D–E 尚未实现。

## B 并发与引用语义

Inference 创建入口先预查 Model 并保存快照，随后提交 service/spec/operation；worker 物化时必须再次读取 Model，已有完成的下载 Job 也不能跳过复查。新增 inference.v1.ModelReferenceService 按实际 ModelVersion UUID 批量查询所有未最终删除实例的保留 spec；待部署、已停止、失败、删除中以及旧 generation 都保守视为引用，不靠 runtime ready 判断。Model 不访问 Inference 表。

Model 删除事务锁定模型（单版本删除锁父模型与版本），再查询 Inference 引用，成功后才软删除。ready 版本读取持有父模型与版本共享锁，创建版本在父模型存在且未删除时持有父模型锁。由此，旧的 worker 读取对应已提交引用；删除检查之后才提交的推理请求，其 worker 复查会读到删除并失败，不得 ready。并发创建入口的预查可能先于删除完成，因此它可能先返回 pending 再由 worker 拒绝。所有锁和远端查询受超时约束；依赖错误回滚，不删除存储对象。该保证依赖 worker 始终在物化前复查模型，必须保留并发测试。
