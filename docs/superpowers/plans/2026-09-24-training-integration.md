# 训练服务集成方案

## 目标

补全 MLOps 全流程，增加**模型训练/微调**能力，形成完整的 训练→注册→部署 闭环。

---

## 当前状态

### ✅ 已有能力
- **模型注册**：ani-model-service（元数据管理、版本控制、MinIO 存储）
- **模型部署**：ani-inference-service（KServe 集成、推理服务）
- **模型导入**：从 Hugging Face/ModelScope 导入外部模型

### ❌ 缺失能力
- 数据集管理
- 模型训练 Job
- 分布式训练编排
- 超参数调优
- 实验追踪

---

## 方案对比

### 方案 1：集成 Kubeflow Pipelines（推荐）

**架构**：
```
Kubeflow Pipelines (训练)
    ↓
ani-model-service (注册)
    ↓
ani-inference-service (部署)
```

**优点**：
- ✅ 成熟的训练编排引擎
- ✅ 可视化 Pipeline 设计
- ✅ 实验追踪 + 血缘关系
- ✅ 支持分布式训练（PyTorchJob、TFJob）
- ✅ 超参数调优（Katib）

**缺点**：
- ⚠️ 部署复杂度高
- ⚠️ 需要学习 Kubeflow DSL
- ⚠️ 资源消耗较大

**集成方式**：

#### 1. Pipeline 最后一步调用 ani-model-service

```python
# training_pipeline.py
from kfp import dsl
import requests

@dsl.component
def train_model(
    dataset: str,
    model_name: str,
    epochs: int,
    output_path: dsl.Output[str]
):
    # 训练逻辑
    import torch
    model = train(dataset, epochs)
    torch.save(model.state_dict(), "/tmp/model.pth")
    output_path.path = "/tmp/model.pth"

@dsl.component
def register_model(
    model_path: str,
    tenant_id: str,
    model_name: str,
    version: str
):
    """注册模型到 ani-model-service"""
    import grpc
    from ani_model_service.api.model.v1 import model_pb2, model_pb2_grpc
    
    # 1. 获取上传 URL
    channel = grpc.insecure_channel("ani-model-service:9000")
    stub = model_pb2_grpc.ModelServiceStub(channel)
    
    response = stub.GetUploadURL(model_pb2.GetUploadURLRequest(
        tenant_id=tenant_id,
        model_id=model_name,
        version=version,
        file_name="model.pth",
        size_bytes=os.path.getsize(model_path)
    ))
    
    # 2. 上传模型文件
    with open(model_path, "rb") as f:
        requests.put(response.upload_url, data=f)
    
    # 3. 创建版本记录
    stub.CreateModelVersion(model_pb2.CreateModelVersionRequest(
        tenant_id=tenant_id,
        model_id=model_name,
        version=version,
        format="pytorch",
        storage_path=response.storage_path
    ))
    
    print(f"Model registered: {model_name}:{version}")

@dsl.pipeline(name="Training Pipeline")
def training_pipeline(
    tenant_id: str = "550e8400-e29b-41d4-a716-446655440000",
    model_name: str = "my-model",
    dataset: str = "s3://datasets/train.jsonl",
    epochs: int = 3
):
    # 步骤 1: 训练
    train_task = train_model(
        dataset=dataset,
        model_name=model_name,
        epochs=epochs
    )
    
    # 步骤 2: 注册到 Model Service
    register_task = register_model(
        model_path=train_task.outputs["output_path"],
        tenant_id=tenant_id,
        model_name=model_name,
        version="v1.0.0"
    )
```

#### 2. 部署 Kubeflow

```bash
# 安装 Kubeflow Pipelines
kubectl apply -k "github.com/kubeflow/pipelines/manifests/kustomize/cluster-scoped-resources?ref=2.0.0"
kubectl wait --for condition=established --timeout=60s crd/applications.app.k8s.io
kubectl apply -k "github.com/kubeflow/pipelines/manifests/kustomize/env/platform-agnostic?ref=2.0.0"

# 配置 Pipeline 调用 ani-model-service
kubectl create configmap ani-integration \
  --from-literal=MODEL_SERVICE_ADDR=ani-model-service.ani.svc:9000
```

---

### 方案 2：自建训练服务 ani-training-service

**架构**：
```
ani-training-service (训练)
    ↓
ani-model-service (注册)
    ↓
ani-inference-service (部署)
```

**优点**：
- ✅ 完全自主控制
- ✅ 与现有架构一致
- ✅ 轻量级部署
- ✅ 定制化能力强

**缺点**：
- ❌ 需要从零开发训练编排
- ❌ 缺少可视化界面
- ❌ 实验追踪需自建

**技术选型**：

```go
// ani-training-service/internal/biz/training/training.go
package training

type TrainingJob struct {
    TenantID    string
    JobID       string
    ModelName   string
    BaseModel   string  // "Qwen/Qwen2.5-0.5B-Instruct"
    Dataset     string  // "s3://datasets/train.jsonl"
    Method      string  // "lora" | "full"
    Hyperparams map[string]interface{}
    Status      string  // "pending" | "running" | "completed" | "failed"
}

type TrainingExecutor interface {
    SubmitJob(ctx context.Context, job TrainingJob) error
    GetJobStatus(ctx context.Context, jobID string) (TrainingJob, error)
}
```

**Kubernetes Job 实现**：

```yaml
# internal/worker/training_executor.go 创建的 Job
apiVersion: batch/v1
kind: Job
metadata:
  name: training-job-abc123
spec:
  template:
    spec:
      containers:
      - name: trainer
        image: registry.ani.io/ani-trainer:v1.0.0
        env:
        - name: BASE_MODEL
          value: "Qwen/Qwen2.5-0.5B-Instruct"
        - name: DATASET
          value: "s3://datasets/train.jsonl"
        - name: OUTPUT_PATH
          value: "/workspace/output"
        - name: LORA_RANK
          value: "8"
        volumeMounts:
        - name: workspace
          mountPath: /workspace
        resources:
          limits:
            nvidia.com/gpu: 1
      volumes:
      - name: workspace
        persistentVolumeClaim:
          claimName: training-pvc-abc123
```

**训练完成后自动注册**：

```python
# cmd/ani-trainer/main.py
import torch
from transformers import AutoModelForCausalLM
from peft import get_peft_model, LoraConfig
import grpc
from ani_model_service.api.model.v1 import model_pb2_grpc

def train():
    # 1. 加载基座模型
    base_model = AutoModelForCausalLM.from_pretrained(os.environ["BASE_MODEL"])
    
    # 2. 配置 LoRA
    lora_config = LoraConfig(r=8, lora_alpha=32, target_modules=["q_proj", "v_proj"])
    model = get_peft_model(base_model, lora_config)
    
    # 3. 训练
    trainer = Trainer(model=model, args=training_args, train_dataset=dataset)
    trainer.train()
    
    # 4. 保存模型
    output_path = "/workspace/output/model.safetensors"
    model.save_pretrained(output_path)
    
    # 5. 注册到 Model Service
    channel = grpc.insecure_channel("ani-model-service:9000")
    stub = model_pb2_grpc.ModelServiceStub(channel)
    
    # 获取上传 URL
    response = stub.GetUploadURL(model_pb2.GetUploadURLRequest(
        tenant_id=os.environ["TENANT_ID"],
        model_id=os.environ["MODEL_NAME"],
        version="trained-" + datetime.now().strftime("%Y%m%d-%H%M%S"),
        file_name="model.safetensors",
        size_bytes=os.path.getsize(output_path)
    ))
    
    # 上传
    with open(output_path, "rb") as f:
        requests.put(response.upload_url, data=f)
    
    print(f"Training completed and model registered!")

if __name__ == "__main__":
    train()
```

---

### 方案 3：集成现有训练平台

如果你已经有其他训练平台（如自建的 GPU 集群、云厂商训练服务），只需要在训练完成后调用 ani-model-service API 注册模型。

**示例：对接阿里云 PAI**：

```python
# pai_callback.py
from alibabacloud_pai_dlc20201203 import models as pai_models
import grpc

def on_training_complete(job_id):
    # 1. 从 PAI 下载训练好的模型
    model_path = download_from_pai(job_id)
    
    # 2. 注册到 ani-model-service
    channel = grpc.insecure_channel("ani-model-service.your-cluster.com:9000")
    stub = model_pb2_grpc.ModelServiceStub(channel)
    
    # 获取上传 URL
    response = stub.GetUploadURL(...)
    
    # 上传模型
    with open(model_path, "rb") as f:
        requests.put(response.upload_url, data=f)
```

---

## 推荐实施路径

### 阶段 1：快速验证（1-2 周）
**目标**：验证训练→注册→部署全流程

1. 手动训练一个小模型（如微调 Qwen2.5-0.5B）
2. 编写脚本调用 `GetUploadURL` + `CreateModelVersion` 注册
3. 验证 ani-inference-service 可以部署该模型

### 阶段 2：集成 Kubeflow（2-4 周）
**目标**：自动化训练流程

1. 部署 Kubeflow Pipelines
2. 编写训练 Pipeline（数据准备 → 训练 → 注册 → 部署）
3. 集成实验追踪

### 阶段 3：优化与扩展（持续）
- 分布式训练（多 GPU、多节点）
- 超参数调优（Katib）
- 模型压缩（量化、剪枝）
- A/B 测试

---

## 需要新增的 API

### ani-model-service 需要增加的接口

#### 1. 批量上传支持（大模型分片上传）

```protobuf
// 当前 GetUploadURL 只支持单文件，大模型需要分片
message GetMultipartUploadURLRequest {
  string tenant_id = 1;
  string model_id = 2;
  string version = 3;
  int64 total_size_bytes = 4;
  int32 part_count = 5;
}

message GetMultipartUploadURLResponse {
  string upload_id = 1;
  repeated UploadPart parts = 2;
}

message UploadPart {
  int32 part_number = 1;
  string upload_url = 2;
}
```

#### 2. 训练任务关联（可选）

```protobuf
// 在 CreateModelVersion 中增加训练任务引用
message CreateModelVersionRequest {
  // ... 现有字段 ...
  string training_job_id = 20;  // 关联的训练任务 ID
  map<string, string> hyperparameters = 21;  // 训练超参数
  string base_model = 22;  // 基座模型
}
```

---

## 风险与考虑

### 1. GPU 资源管理
- 训练需要独占 GPU，需要配额管理
- 建议使用 Kubernetes Resource Quota

### 2. 数据集管理
- 当前架构没有数据集服务
- 短期方案：直接使用 S3/MinIO 存储数据集
- 长期方案：考虑增加 ani-dataset-service

### 3. 成本控制
- 训练可能消耗大量资源
- 建议：
  - 设置训练任务超时（默认 24 小时）
  - 按租户限制并发训练任务
  - 监控 GPU 使用率

---

## 参考资料

- [Kubeflow Pipelines 文档](https://www.kubeflow.org/docs/components/pipelines/)
- [Kubeflow Training Operators](https://www.kubeflow.org/docs/components/training/)
- [LLaMA-Factory (微调工具)](https://github.com/hiyouga/LLaMA-Factory)
- [vLLM (推理引擎)](https://github.com/vllm-project/vllm)

---

## 下一步行动

1. **决策训练方案**：Kubeflow vs 自建 vs 对接现有平台
2. **验证训练→注册流程**：手动训练一个模型并注册
3. **评估资源需求**：GPU 数量、存储容量
4. **制定实施计划**：排期、里程碑
