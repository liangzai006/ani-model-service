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
