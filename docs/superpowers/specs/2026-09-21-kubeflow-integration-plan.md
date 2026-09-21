# Kubeflow 接入方案

日期：2026-09-21  
状态：提案  
适用范围：`ani-model-service`、`ani-inference-service`、Kubernetes、APISIX

## 结论

当前不安装完整 Kubeflow，也不让用户直接通过 Kubeflow Dashboard 创建模型或推理服务。

保留 ANI 作为产品业务控制面：

- IAM、tenant、actor、mTLS 和审计；
- Model 目录、ModelVersion、制品 checksum 和导入任务；
- Inference 的 create、stop、restart、update、delete、operation、generation 和恢复；
- Quota 的 reserve、confirm、release；
- Publication 和 APISIX `HTTPRoute`。

Kubeflow 只作为 Kubernetes 上的执行层，按需要接入具体子项目：

1. 需要标准化推理运行时后，先接 KServe；
2. 需要训练或微调后，再接 Kubeflow Trainer；
3. 需要多步骤训练、量化、评测和注册流程后，再接 Kubeflow Pipelines；
4. 需要超参数搜索时，再接 Katib。

Kubeflow 官方支持独立部署子项目，因此不要求一次性安装完整发行版。[Kubeflow 安装方式](https://www.kubeflow.org/docs/started/installing-kubeflow/)

## 现有系统边界

```text
外部 API / Gateway
        |
        v
IAM -> ANI Model / ANI Inference -> PostgreSQL desired state
             |             |
             |             +--> 当前 Inference Controller
             |                       +--> Runtime Deployment/LWS/Service
             |                       +--> Materializer Job/PVC
             |                       +--> APISIX HTTPRoute
             |
             +--> Import Job -> MinIO -> ModelVersion ready
```

现有服务的职责不应因为引入 Kubeflow 而重复：

| 领域 | 业务权威 | Kubeflow 是否接管 |
| --- | --- | --- |
| 租户、身份、权限 | ANI IAM 和服务间身份 | 否 |
| 模型目录、版本、checksum | `ani-model-service` | 否 |
| 模型导入 | Model 导入任务和独立 Kubernetes Job | 现阶段否 |
| 推理服务生命周期 | `ani-inference-service` | 否 |
| 推理 Deployment/Service | 当前 Inference Runtime Executor，未来可选 KServe | 可选 |
| 对外路由 | APISIX + `HTTPRoute` | 否 |
| 训练/微调 | 当前尚未建设 | Trainer/KFP |
| 超参数优化 | 当前尚未建设 | Katib |

同一个 Deployment、Service 或模型下载任务只能有一个 controller 负责。ANI Controller 和 KServe 不能同时管理同一套运行资源。

## 目标架构

### 第一阶段：KServe 作为可选推理后端

保留当前 ANI API 和 PostgreSQL 状态模型，在 `ani-inference-service` 增加一个 `KServeRuntimeProvider`：

```text
Inference API
    |
    +--> IAM / tenant check
    +--> Quota reserve
    +--> PostgreSQL operation + desired state
    |
    v
ANI Inference Controller
    |
    +--> KServeRuntimeProvider
             |
             +--> serving.kserve.io InferenceService
             |       或 LLMInferenceService
             |
             +--> KServe Predictor Service
                     |
                     +--> APISIX HTTPRoute
                             |
                             +--> 外部 HTTP /v1/*
```

KServe 负责推理运行时的 Kubernetes 资源、健康状态和运行时生命周期；ANI 仍负责业务状态、租户、配额、操作 fencing 和对外发布。KServe 的 `InferenceService` 是其主要模型服务抽象，支持模型服务的资源、版本和流量管理。[KServe 资源模型](https://kserve.github.io/website/docs/concepts/resources)

第一阶段使用 KServe Standard 部署模式。KServe 的外部入口不作为公开入口，APISIX 继续是唯一的外部入口；KServe 只提供集群内 Predictor Service。这样可以避免 KServe 自己的入口和 APISIX 产生双入口。

### 第二阶段：Trainer 用于训练和微调

训练不是 Model Import 的替代路径，而是一个新的业务能力：

```text
ANI Training API
    |
    +--> IAM / tenant check
    +--> Quota reserve GPU/CPU/PVC
    +--> PostgreSQL TrainingTask
    |
    v
Kubeflow Trainer TrainJob
    |
    +--> prepare data / base model
    +--> distributed fine-tune
    +--> evaluate
    +--> upload artifact to MinIO
    |
    v
ANI Model API
    |
    +--> create ModelVersion
    +--> persist checksum and artifact
    +--> mark ready
```

Kubeflow Trainer 面向 Kubernetes 原生的多节点、多 GPU 训练和大模型微调，并提供 `TrainJob` 和 Runtime API。[Kubeflow Trainer](https://www.kubeflow.org/docs/components/trainer/overview/)

### 第三阶段：Pipelines 用于多步骤流程

只有出现下面这类流程时才接 KFP：

```text
数据准备 -> 训练 -> 评测 -> 量化 -> 安全扫描 -> 上传 -> 注册 ModelVersion
```

Pipelines 负责步骤编排、参数传递、重试、缓存和制品流转；它不负责 ANI 的租户权限、Model 目录或 Publication。KFP Pipeline 的每个步骤最终会在 Kubernetes 中以独立容器运行。[Kubeflow Pipelines](https://www.kubeflow.org/docs/components/pipelines/concepts/pipeline/)

当前的模型导入只有“下载、校验、上传”一个主要执行单元，继续使用现有独立 Import Job 更简单，不迁移到 KFP。

## KServe 接入设计

### 运行时适配器

在 `ani-inference-service` 增加基础接口，具体名称可以沿用现有 Runtime Executor 的风格：

```text
Apply(ctx, RuntimeSpec) -> RuntimeHandle
Observe(ctx, RuntimeHandle) -> RuntimeObservation
Stop(ctx, RuntimeHandle) -> error
Restart(ctx, RuntimeHandle) -> error
Delete(ctx, RuntimeHandle) -> error
```

`KServeRuntimeProvider` 只负责 KServe CRD 的 Apply/Get/Delete 和状态转换，不读取或写入 ANI 的 PostgreSQL 表。现有 operation fencing、generation、UID 和 resourceVersion 继续由 Inference 服务控制。

KServe CRD 使用独立的 API group，例如 `serving.kserve.io`；现有 ANI CRD 使用自己的 `ani.kubercloud.com` group。两者可以并存，但不能让两个 controller 操作同一 Deployment。

### 模型制品

Model 服务继续是模型制品唯一权威。优先复用 Inference 现有 Materializer 的结果：

1. Materializer 从 Model 服务取得制品；
2. 校验 checksum；
3. 解压到指定 PVC 的模型目录；
4. KServe 通过 PVC 路径启动 Predictor。

这样不会让 KServe 再下载一次几百 GiB 模型。当前制品是 `.tar`，必须先验证解压后的目录是否包含 vLLM 所需的 `config`、tokenizer 和权重文件；不能直接把 `.tar` URL 当成 vLLM 模型目录。

如果后续改用对象存储 URI，则只给 KServe ServiceAccount 提供受限、短期或只读凭据，不把签名 URL 写入长期 CR 或日志。

### API 路径和入口

当前外部 API 使用 `/v1`。KServe/vLLM runtime 的默认路径和服务端口需要在适配器中固定，并用单模型 smoke test 验证：

```text
GET  /v1/models
POST /v1/chat/completions
```

APISIX `HTTPRoute` 只指向 KServe Predictor Service。Publication Withdraw 时先撤销路由、确认不再接受新请求，再执行 KServe runtime 的停止或删除。

## 租户、IAM 和 Quota

Kubeflow Profile/Namespace 是 Kubernetes 资源隔离边界，不等于 ANI 的 tenant UUID。不能用 Profile 名称代替业务租户身份，也不能让用户绕过 ANI 直接提交 KServe 或 KFP 资源。

接入要求：

- 外部请求先经过 ANI IAM，得到可信 `Principal.TenantID`；
- ANI 在 PostgreSQL 中保存业务任务和目标资源；
- ANI 根据租户映射选择已授权的 Kubernetes Namespace 和 ServiceAccount；
- 创建 KServe/Trainer/KFP 资源时写入租户标签、request ID 和 operation ID；
- GPU、CPU、PVC 等资源创建前由 ANI Quota Reserve；
- 创建失败、删除或终止时由 ANI Quota Release；
- 成功进入可用状态后由 ANI Quota Confirm；
- Kubeflow Dashboard 只面向内部管理员，初期不作为租户公网入口。

KFP 的多用户隔离依赖 Kubernetes Namespace 和 Kubeflow Profile；官方文档也说明 Profile 的隔离基础仍是 Kubernetes Namespace/RBAC。[KFP 多用户隔离](https://www.kubeflow.org/docs/components/pipelines/operator-guides/multi-user/)、[Kubeflow Profiles](https://www.kubeflow.org/docs/components/central-dash/profiles/)

## 部署策略

### 不采用的方案

- 不直接安装完整 Kubeflow Community Distribution；它会同时引入 Dashboard、Profile、Pipelines、Trainer、Katib、KServe、认证、存储和大量 CRD/controller，当前项目还没有对应的训练需求。
- 不让 Kubeflow Dashboard 直接创建 ANI Model 或 Inference 资源。
- 不把当前 Model Registry 替换成 Kubeflow 的元数据或模型目录。这样会产生两套版本、权限和删除语义。
- 不把当前 Import Job 改成 KFP Pipeline。现有 Job 已满足下载、bucket 创建、PVC、上传和 checksum 需求。

### 推荐部署顺序

1. 在独立测试 Namespace 安装固定版本的 KServe CRD/controller。
2. 配置一个只读 Model artifact ServiceAccount 和 MinIO 访问策略。
3. 手工部署一个已 ready 的小模型，验证 KServe Predictor、vLLM API 和 APISIX 路由。
4. 在 Inference 服务实现 `KServeRuntimeProvider`，先只接 create/observe/delete。
5. 补 stop/restart/update 和旧 generation fencing。
6. 完成 create → ready → publish → request → withdraw → stop/restart/update/delete 验收。
7. 需要训练时再安装 Trainer；需要多步骤工作流时再安装 KFP；需要超参搜索时再安装 Katib。

## 验收标准

### KServe 推理后端

- [ ] KServe CRD/controller 与现有 ANI CRD 不冲突。
- [ ] 创建一个 Inference 业务资源只产生一个受控的 KServe 资源。
- [ ] KServe 产生 Predictor Service，APISIX `HTTPRoute` 状态为 `Accepted=True`、`ResolvedRefs=True`。
- [ ] `GET /v1/models` 和一次真实推理请求成功。
- [ ] stop 后 APISIX 不再接收新请求，运行时资源按策略停止。
- [ ] restart 恢复原 generation，不产生重复路由。
- [ ] update 只切换目标版本，旧 generation 不会覆盖新 generation。
- [ ] delete 先撤销 Publication，再删除 KServe 资源和物化资源。
- [ ] Model artifact checksum、租户和资源配额均能在审计记录中追溯。

### Trainer/KFP

- [ ] ANI 能为任务保留租户、请求、配额和版本信息。
- [ ] Trainer/KFP 任务失败、重试、取消和 worker 重启后状态可恢复。
- [ ] 训练输出只能通过 Model API 注册为新的 ModelVersion。
- [ ] 训练制品和 KFP metadata 与租户边界一致。
- [ ] 不允许用户通过 Kubeflow API 绕过 ANI IAM/Quota。

## 回滚方案

KServe 接入必须通过运行时配置或 provider 选择启用。回滚时：

1. 停止创建新的 KServe 资源；
2. 保留 PostgreSQL desired state 和 operation 记录；
3. 将新的 Inference 资源切回现有 Runtime Executor；
4. 撤销遗留 APISIX 路由；
5. 按 generation 和 UID 删除孤立的 KServe 资源；
6. 不删除 Model 制品和 ModelVersion。

## 第一阶段开发任务

1. 在 `ani-inference-service` 定义 `KServeRuntimeProvider` 端口和状态映射。
2. 生成或引入 KServe CRD 客户端，使用 typed API；只有确实没有 typed API 的资源才使用动态客户端。
3. 增加 KServe Standard `InferenceService` 的 vLLM 资源模板。
4. 明确 Materializer PVC 到 KServe `storageUri` 的目录契约。
5. 固定 `/v1` 路径、health probe、Service port 和 APISIX backend port。
6. 增加 create/observe/delete 单元测试和 Kubernetes envtest。
7. 在真实集群完成单模型 smoke，再实现 stop/restart/update/delete。

## 参考资料

- [Installing Kubeflow](https://www.kubeflow.org/docs/started/installing-kubeflow/)
- [Kubeflow Pipelines](https://www.kubeflow.org/docs/components/pipelines/)
- [Kubeflow Trainer](https://www.kubeflow.org/docs/components/trainer/)
- [KServe Introduction](https://www.kubeflow.org/docs/ecosystem/kserve/introduction/)
- [KServe Resources](https://kserve.github.io/website/docs/concepts/resources)
- [KServe Administrator Guide](https://kserve.github.io/website/docs/admin-guide/overview)
