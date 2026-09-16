# Model Service 设计

## 目标

建立与 `ani-inference-service` 同级的独立 Model 服务，使用固定
`ani-kratos-layout` commit `fd18422211c741dd5242d2992c3d307d522aa28c`，独立构建、版本、进程、数据库 migration 和 gRPC 契约。

Model 服务拥有模型目录、模型版本、模型制品和模型导入任务。Inference 只通过版本化 gRPC 查询 Model，不访问 Model 数据库，也不复制 Model 表。

## 存储边界

Model 服务与 Inference 共用现有 PostgreSQL 实例和数据库，但使用`public` schema；Inference 的 `public.inference_*` 表不被 Model 服务写入。开发阶段暂时共用同一个数据库访问账号和权限；后续再拆分凭据与 schema 权限。无论凭据是否共用，禁止跨服务数据库外键、共享写表和共享事务。

第一阶段表：

- `public.models`：租户范围的模型逻辑实体。
- `public.model_versions`：模型的具体版本和版本状态。
- `public.model_artifacts`：制品地址、格式、大小、SHA256 和加密元数据；摘要和内容引用不可变，当前生效制品由版本记录明确指向。
- `public.model_import_tasks`：上传或远程导入的持久异步任务。

第一阶段不创建 `model_materializations`。推理 Pod 中的下载、挂载、模型加载、runtime ready 和调用健康属于 Inference。

所有租户业务表显式保存 `tenant_id`，不用 RLS；sqlc 查询必须显式带租户条件，租户范围的主键、唯一约束和复合外键保持一致。数据库迁移和跨租户 worker 入口分离管理；当前开发环境运行凭据暂与 Inference 共用，并标记为后续隔离项。

## gRPC 契约

业务入口固定为 `客户端 HTTP → 独立 Gateway → Model gRPC`；Model 负责模型业务处理。
Gateway 是独立仓库/进程的统一入口，负责 HTTP 请求接入和协议转发；本仓库不实现
Gateway，不注册 Model 业务 HTTP 路由。Model 保留 layout 的 Kratos gRPC transport。
独立 admin HTTP listener 仅提供 `/healthz`、`/readyz`、`/metrics` 运维端点。

Gateway 转发不改变 Model 的身份信任规则：Model 仍须验证可信调用者与委托范围，
不能直接信任客户端填写或未经验证转发的 tenant/actor header。
Storage 集成以其正式版本化契约为依据，不能由 Model 单方面假定 REST 路径；
正式契约未核验前，Storage 联调保持 `not_verified`。

保留旧 ANI Model 原型的兼容方法和语义：

```text
CreateModel
GetModel
ListModels
DeleteModel
GetModelVersion
ListModelVersions
CreateModelVersion
GetUploadURL
ImportModel
GetModelDownloadURL
```

`GetModelVersion` 是 Inference 的内部服务间查询；`GetModelDownloadURL` 返回短期制品下载地址。请求可以保留 `tenant_id` 以兼容旧契约，但服务必须从可信 IAM/Workload 上下文取得直接调用者和适用委托范围，并将字段仅作为交叉校验；普通 Header 不能作为身份凭证。写操作使用调用方提供的幂等键。

在 `ModelVersion` 上只做向后兼容的可选新增：

```text
engine_type
startup_command
startup_args
```

它们表示 Model 服务维护的受信默认运行建议。Model 服务只允许经过 engine allowlist 和参数校验的配置；Inference 可以按自身契约合并允许的覆盖，并把最终 `engine_runtime`/`command_argv` 保存到自己的 spec，不能接受任意租户命令。

## 状态和异步任务

模型版本状态为：

```text
pending → importing → ready
                    ↘ error
```

创建模型版本、创建导入任务和幂等结果在同一本地事务中提交。导入 worker 从 PostgreSQL 扫描 due task，并使用 `lease_token`、attempt、CAS 和超时恢复；旧 worker 或旧版本结果不得覆盖当前任务。进程重启、消息丢失和调用方不查询时仍须继续恢复。远程源下载、对象存储上传和 SHA256 校验结果必须持久化；不能用内存队列作为唯一任务来源。对象存储、文件系统、加密和下载能力通过 Storage/密钥服务 adapter 使用，Model 不拥有这些基础设施。

普通查询可以返回非 `ready` 版本；只有状态为 `ready` 且制品摘要完整的版本，才允许作为 Inference 部署输入。删除模型采用软删除，并在存在有效 Inference 引用时拒绝删除。删除前通过版本化的 Inference 引用查询协议检查；查询超时或错误必须拒绝删除，不能按无引用处理。该检查具备幂等、重试和明确的 `unknown` 结果，不使用数据库外键。

## 领域职责

Model 服务负责模型元数据、版本、制品可用性、导入任务、下载授权和相关审计。资源变更、任务受理和幂等结果与审计事件在本地事务中提交；事件至少包含 tenant、Actor、Workload、RequestID、operation/task ID、动作、before/after 状态和错误分类。它不创建或观察 Inference Deployment/LWS/Service，不管理推理配额，不判断推理 Pod 是否 ready，也不发布推理 endpoint。

Inference 负责把 Model 返回的制品和默认启动配置应用到自己的 runtime，并持续观察模型加载和调用健康。

## 验证范围

正式进程的 `/readyz` 必须检查 PostgreSQL、Storage adapter 和导入 worker；依赖未配置时保持未就绪。必须验证：sqlc 租户隔离、真实 PostgreSQL migration、导入任务重启恢复、SHA256 校验、幂等重放、`GetModelVersion` 与 `GetModelDownloadURL` 的权限和过期语义、删除引用检查、启动命令 allowlist，以及 Inference 使用正式 Model gRPC 的端到端联调。

不能用 fake Model provider、固定模型或固定配额证明正式链路完成。
