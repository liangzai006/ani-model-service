# Model 与 Inference 字段联动

## 创建顺序

1. 创建 ModelVersion 时保存引擎类型、镜像和完整启动命令。
2. 前端选择一个 `model_version_id`，调用 `GetModelVersion` 或 `ListModelVersions`。
3. 将版本返回的默认值填入推理创建表单。
4. 提交推理创建请求时仍携带最终值，用户可以在本次推理中临时修改。

## 字段映射

| ModelVersion | Inference 创建请求 | 说明 |
| --- | --- | --- |
| `engine_type` | `engine.type` | 例如 `vllm` |
| `engine_image` | `engine.image` | 完整镜像引用 |
| `startup_command` | `engine.command` | 完整命令字符串 |

推理创建还需要业务资源参数：`model_version_id`、`name`、`cpu`、`memory` 和
`replicas`。GPU 场景另外提交 `resource.gpu`；未提交 GPU 时不会增加 GPU 资源。

示例：

```json
{
  "data": {
    "idempotency_key": "req-001",
    "name": "qwen-inference",
    "model_version_id": "version-uuid",
    "served_model_name": "qwen2.5-1.5b-instruct",
    "engine_type": "vllm",
    "engine_image": "registry.example/vllm:latest",
    "command": "vllm serve /models --served-model-name qwen2.5-1.5b-instruct",
    "cpu": "8",
    "memory": "32Gi",
    "replicas": 1
  }
}
```

`command` 必须是调用方提供的完整字符串。Gov 和 Inference 会原样转发和保存，
不会拆分成参数数组、追加参数或根据模型名称拼接命令。推理表单的临时修改只影响
当前推理，不会回写 ModelVersion。

如果历史 ModelVersion 的三个字段为空，前端不能自动填充，必须在创建推理时提供
完整值；Model 服务不会猜测引擎或启动命令。

Model 和 ModelVersion 列表统一使用 `page`、`pageSize`，响应返回 `items`、`total`。

## 从 ModelScope/Hugging Face 导入模型

外部模型搜索和本地模型目录是两个动作。前端通过 Gov 的
`GET /api/v1/models/search`（Model 服务内部对应
`SearchExternalModels` gRPC）查询候选；Model 服务只允许 `modelscope` 和
`huggingface` 两个 provider。请求字段如下：

| 字段 | 含义 |
| --- | --- |
| `source` | `modelscope` 或 `huggingface` |
| `keyword` | 模型名称或关键词，必填 |
| `owner` | 可选的组织/作者过滤 |
| `task` | 可选的任务过滤 |
| `library` | 可选的框架/库过滤 |
| `page`、`pageSize` | 页码分页；`page` 从 1 开始，`pageSize` 默认为 20，范围为 1～100 |

搜索响应返回 `items` 和 `hasMore`，不承诺 `total`。搜索结果是临时候选，不会写入 Model 表、ModelVersion 表，也不会启动下载。候选
至少包含 `source`、`repo_id`、`name`、`display_name`、`description`、`revision`、
`size_bytes`、`tags` 和 `updated_at`。ModelScope 没有返回 revision 时使用 `master`，
Hugging Face 没有返回 revision 时使用 `main`。

前端确认候选后按以下顺序提交：

1. `CreateModel`：用候选的 `repo_id` 作为稳定的 `model_id`，用候选名称作为展示
   名称；这个动作才创建租户本地 Model 记录。
2. `CreateModelVersion`：把确认的 revision 作为版本标识，并提交版本的引擎默认值
   `engine_type`、`engine_image`、`startup_command`。这些值由前端或模型版本配置
   提供，搜索服务不会从 provider 推断，更不会拼接启动命令。
3. `ImportModel`：提交候选的 `source`、`repo_id`、`revision`，以及上一步返回的
   `model_id`、`model_version_id` 和幂等键。导入的是整个 provider 仓库，不是搜索
   结果中的某一个文件。

导入任务创建后通过现有 `GetImportTask` 轮询状态。只有确认操作进入这一步才会
创建导入任务并下载模型；用户取消或未确认候选时不会产生本地记录和下载任务。
