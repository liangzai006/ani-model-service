# AI 服务模块核对：Model 与 Inference

日期：2026-09-17。

## 范围与依据

本次只核对 `/root/design/原型9.8` 中的 AI 服务：模型仓库、推理服务、限流与访问策略。
不使用旧 ANI 原型替代本次原型，不扩展到知识库、存储等其他产品模块。
IAM/Gateway 按用户决定后置，不作为本次 AI 服务开发的阻塞项。

原型入口：

- `/root/design/原型9.8/index.html:658`：模型仓库、推理、策略页面定义。
- `/root/design/原型9.8/market-detail.md:320`：模型仓库。
- `/root/design/原型9.8/market-detail.md:449`：推理服务与限流策略。
- `/root/design/原型9.8/models-link.js:24`：Chat/vLLM、Embedding/TEI 的原型映射。

两个仓库已有未提交修改，本次只读取现有实现并追加本记录，不覆盖既有补丁。
历史 `docs/execution/status.md` 存在滞后条目；不能把其中旧的联调未通过结论当作现状，
也不能把单模型联调成功扩大为整个 AI 服务模块已完成。

## 本次实际验证

保留的测试实例：

- Namespace：`ani-model-inference-e2e-retry-20260916`。
- Deployment：`smollm2-live-20260916-final`。
- Kubernetes 只读回查：`desired=1 ready=1 available=1`。
- 端点：`http://10.10.1.67:30325`。
- 模型调用名：`smollm2-135m`。

本次实际 completion 请求返回 HTTP 200：

```sh
curl --fail-with-body --max-time 35 \
  http://10.10.1.67:30325/v1/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"smollm2-135m","prompt":"The capital of France is","max_tokens":8,"temperature":0}'
```

返回文本：` Paris. Paris is the largest city in`。
响应 ID：`cmpl-e6e3c54cfb13400bb76b0f380998144d`；输入 5 tokens，输出 8 tokens。

本次实际 Chat 请求返回 HTTP 200：

```sh
curl --fail-with-body --max-time 35 \
  http://10.10.1.67:30325/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"smollm2-135m","messages":[{"role":"user","content":"Say hello in one short sentence."}],"max_tokens":12,"temperature":0}'
```

返回文本：`Hello!`。
响应 ID：`chatcmpl-8e824b71ea214e17aacaf22c2cd3477e`；输入 37 tokens，输出 3 tokens。

宿主机定向回归均以退出码 0 完成：

```sh
cd /root/kubercon/ani-model-service
go test -p 2 ./internal/service ./internal/biz/model ./internal/data/importer ./internal/worker -count=1

cd /root/kubercon/ani-inference-service
go test -p 2 ./internal/service ./internal/data/model ./internal/data/invocation ./internal/data/development ./internal/biz/inference -count=1
```

本轮未额外配置或单独验收真实 PostgreSQL/双进程 E2E；包测试通过不代替这些证据。
未启停、更新或删除成功部署，未重新读取 operation 当前状态；历史完整部署证据见本仓库 `progress.md`。

## 确认的功能缺口和实现限制

### 1. Model 删除入口尚不可用

`internal/service/model.go:184` 的 DeleteModel 在引用检查器为空时返回
`INFERENCE_REFERENCE_CHECK_UNAVAILABLE`。组合根 `cmd/ani-model-service/main.go`
配置了 catalog、artifact、audit、worker，但没有注入 `SetInferenceReferenceChecker`；
该 setter 当前仅在测试中调用。

影响：即使模型无推理引用，正常进程的模型删除请求仍会被拒绝。
应补版本化的 Inference 引用查询和 Model 客户端接线，不能通过放开检查实现删除。
另外 Model API 尚无删除单个版本接口，原型“删除版本”未被覆盖。

### 2. Model 列表筛选、分页和展示字段不完整

`internal/service/model.go:162` 的 ListModels 只传递 tenant、status、limit；
Proto 已声明的 keyword、source、capability、cursor 未消费，响应没有填写分页 meta。
`ListModelVersions` 同样只处理 limit，没有游标续页。

`toProtoModel` 未填充 created_at、updated_at、versions；`toProtoVersion` 未填充
size_bytes、created_at 等字段。原型最新版本、更新时间、版本大小等展示不能据此视为完整。

### 3. 完整远程模型导入到部署尚未形成通用流程

`internal/data/importer/http_source.go` 的 HF/ModelScope adapter 只获取显式
`repo_id#file` 指定的一个文件，不会枚举并组织整个模型仓库所需的权重、配置、tokenizer。

此前成功模型由 `scripts/prepare-smollm-bundle.py` 准备完整 tar，再通过正常上传 API
导入。这证明上传制品可部署，不等于用户只填写 HF 仓库即可完成完整模型的导入与部署。

ImportModel 返回任务 ID，但当前 Model gRPC 没有独立任务查询/重试接口。
原型基础失败反馈和重试仍需实现；暂停、断点续传等标记为规划的高级任务能力不在此直接承诺。

### 4. 模型物化与引擎存在明确限制

Inference `internal/data/model/materializer.go:39` 限定配置租户与
`ani-model-inference-e2e-` namespace，制品必须为 tar，运行模式必须为 Deployment。
`download_bundle.py:14` 将下载与解包分别限制为 512 MiB；PVC 请求固定 1 Gi。
`internal/data/kubernetes/model_volume.go` 的模型挂载只接受 vLLM。

Model `internal/biz/model/model.go` 的引擎枚举允许 vllm/sglang/tgi，但不含原型中的 TEI；
completion 探针仅覆盖文本生成，不支持 embeddings 验证。
因此当前证据仅覆盖小型 vLLM 文本模型，不代表大模型、Embedding、其他引擎可部署。

### 5. 开发身份与执行链接线仍耦合

Inference `cmd/ani-inference-service/development_runtime.go` 在开发开关开启时才装配
Model materializer、quota、publication、completion probe；默认路径未提供相应替代装配。
关闭开发身份开关并不能直接得到可部署的普通运行模式。

开发 quota 还限定单个运行单元、最多 1 GPU，以及 CPU/内存上限；
这些是专用联调限制，不是原型要求的通用副本配置能力。
后续应在保留显式开发身份的前提下，明确 AI 执行链的正常配置与资源边界；
不需要因此提前开发 IAM/Gateway 或其他业务模块。

### 6. 更新模型版本已补齐；真实生命周期仍待验收

Inference 已实现 Create/Get/List/Update/Start/Stop/Restart/Delete 和 operation 查询。
本轮相关定向回归通过，不能再说“启停、变配都没做”。

此前 `api/inference/v1/inference.proto` 的 UpdateInferenceServiceRequest 没有
model_version_id，领域 UpdateInput 也没有该字段；该缺口已在 2026-09-18 补齐。
Update 现在通过 Model gRPC 重新读取 ready 快照，并将版本和制品事实写入下一代
Inference spec，不能以直接覆盖制品引用代替版本变更。

启停、重启、变配和删除的真实模型生命周期，以及 worker 在操作中断后的恢复，
本轮没有执行；已有单元/数据库级历史证据不能替代这组真实运行验收。

### 7. 限流、日志、监控、事件和用户调用测试接口尚未齐备

当前 Inference gRPC 只有资源生命周期和 operation 查询，没有策略 CRUD/绑定、
运行实例日志、业务指标、事件或用户调用测试 RPC。
管理端 `/metrics`、内部 completion probe 和直接访问引擎成功，各自只覆盖部分能力，
不等于原型各页面所需的产品接口已完成。

QPS/并发实际执行与 429 计数还需实现和验证。策略属于当前 AI 服务范围，
IAM/API Key 正式授权链继续后置。

## 还需补的运行一致性验收

- 下载 Job 进入 Failed 后，materializer 每次只返回非 ready 原因，没有恢复该终态 Job 的路径；
  Kubernetes Job 自身的两次重试不等于产品层失败后的人工重试。
- PVC/Job/下载 Secret 没有设置 OwnerReference/TTL；当前 runtime 删除逻辑针对持久化的
  runtime/endpoint binding。需补删除、更新时模型物化资源的保留/回收策略及真实验收，
  不能未经确认删除当前保留资源。
- Model 资源/导入任务写入与 `recordAudit` 是分开的调用；审计失败可能在业务数据已写入后
  向客户端返回错误。需补同一业务事务的审计一致性；本轮仅做代码核对，未注入数据库失败。

## 建议修复顺序

1. 修 Model 可见功能：列表筛选/分页与响应字段、模型/版本删除及引用保护、导入任务查询。
2. 补完整模型的导入和制品组织，明确容量与引擎支持；让部署不再限定本次样例。
3. 对已实现的 ModelVersion 切换验收真实启停、重启、变配、更新、删除与故障恢复，
   并解除只支持样例引擎/容量的运行限制。
4. 补原型调用测试、日志/指标/事件，以及限流策略和实际 429 验证。

模型评测、模型优化、血缘、协作审批等明确标“规划”的高级 Tab 单独列为规划，
不能为了宣布基础 AI 模块完成而假装已有，也不应与当前基础功能修复混在一起。

本次没有修改业务源码，没有 commit/push，没有变更现有 Kubernetes 资源。
