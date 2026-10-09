# Tokenizer 配置增强设计

## 背景

当前 `ModelVersion` 只校验模型格式（`safetensors`/`gguf`/`pytorch`），但不处理 tokenizer 配置。推理服务部署时需要：

1. **Tokenizer 文件**：`tokenizer.json`, `tokenizer_config.json`, `special_tokens_map.json`
2. **模型配置**：`config.json`（架构信息）
3. **词表文件**：`vocab.txt` (BERT) 或嵌入在 tokenizer.json 中

如果这些文件缺失，推理服务启动失败，但当前没有前置校验。

---

## 方案对比

### 方案 A：通过 startup_args 传递（当前可用）

```protobuf
message CreateModelVersionRequest {
  string engine_type = 12;      // "vllm"
  string startup_command = 13;  // "vllm"
  repeated string startup_args = 14;  // ["--tokenizer", "/mnt/models"]
}
```

**优点**：
- ✅ 无需修改 proto
- ✅ 灵活，引擎自己处理

**缺点**：
- ❌ 没有类型约束
- ❌ 无法提前校验文件
- ❌ 不同引擎参数不同（vllm vs tgi vs llama.cpp）

---

### 方案 B：新增专用字段（推荐）

#### 1. 扩展 proto

```protobuf
message ModelVersion {
  // ... 现有字段 ...
  
  // 新增：tokenizer 配置
  TokenizerConfig tokenizer = 16;
  
  // 新增：模型元数据（从 config.json 解析）
  ModelMetadata metadata = 17;
}

message TokenizerConfig {
  string type = 1;              // "auto" | "custom" | "none"
  string path = 2;              // tokenizer 路径（相对于模型根目录）
  bool trust_remote_code = 3;   // 是否允许自定义 tokenizer
  int32 vocab_size = 4;         // 词表大小（可选，用于校验）
  
  // 特殊 token（可选）
  map<string, string> special_tokens = 5;  // {"bos": "<s>", "eos": "</s>"}
}

message ModelMetadata {
  string architecture = 1;      // "QWenLMHeadModel"
  int32 hidden_size = 2;        // 隐藏层维度
  int32 num_layers = 3;         // 层数
  int32 num_attention_heads = 4;  // 注意力头数
  string dtype = 5;             // "float16" | "bfloat16"
}
```

#### 2. 导入时自动提取

在 `ImportExecutor` 或 Kubernetes Job 中增加：

```go
// internal/worker/import_executor.go

type ModelValidator struct {
    Storage storage.Port
}

func (v *ModelValidator) ValidateModel(ctx context.Context, tenantID, objectRef string) (*ValidationResult, error) {
    // 1. 下载并解压模型包
    tmpDir := "/tmp/model-validation"
    if err := v.extractModel(ctx, tenantID, objectRef, tmpDir); err != nil {
        return nil, err
    }
    defer os.RemoveAll(tmpDir)
    
    result := &ValidationResult{}
    
    // 2. 检查必需文件
    requiredFiles := []string{"config.json"}
    for _, file := range requiredFiles {
        if !fileExists(filepath.Join(tmpDir, file)) {
            result.Errors = append(result.Errors, fmt.Sprintf("missing required file: %s", file))
        }
    }
    
    // 3. 解析 config.json
    configPath := filepath.Join(tmpDir, "config.json")
    if fileExists(configPath) {
        config, err := parseConfigJSON(configPath)
        if err != nil {
            result.Warnings = append(result.Warnings, fmt.Sprintf("failed to parse config.json: %v", err))
        } else {
            result.Metadata = config
        }
    }
    
    // 4. 检测 tokenizer
    tokenizerFiles := []string{"tokenizer.json", "tokenizer_config.json"}
    foundTokenizer := false
    for _, file := range tokenizerFiles {
        if fileExists(filepath.Join(tmpDir, file)) {
            foundTokenizer = true
            break
        }
    }
    
    if !foundTokenizer {
        result.Warnings = append(result.Warnings, "no tokenizer files found, inference may fail")
    } else {
        // 解析 tokenizer 配置
        tokenizerConfig, err := parseTokenizerConfig(filepath.Join(tmpDir, "tokenizer_config.json"))
        if err == nil {
            result.TokenizerConfig = tokenizerConfig
        }
    }
    
    return result, nil
}

type ValidationResult struct {
    Errors          []string
    Warnings        []string
    Metadata        *ModelMetadata
    TokenizerConfig *TokenizerConfig
}

type ModelMetadata struct {
    Architecture string
    HiddenSize   int
    NumLayers    int
    VocabSize    int
    DType        string
}

type TokenizerConfig struct {
    Type           string
    VocabSize      int
    SpecialTokens  map[string]string
}

func parseConfigJSON(path string) (*ModelMetadata, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    var config map[string]interface{}
    if err := json.Unmarshal(data, &config); err != nil {
        return nil, err
    }
    
    return &ModelMetadata{
        Architecture: getString(config, "architectures", 0),
        HiddenSize:   getInt(config, "hidden_size"),
        NumLayers:    getInt(config, "num_hidden_layers"),
        VocabSize:    getInt(config, "vocab_size"),
        DType:        getString(config, "torch_dtype"),
    }, nil
}
```

#### 3. 在 CreateModelVersion 时校验

```go
// internal/service/model.go

func (s *ModelService) CreateModelVersion(ctx context.Context, in *modelv1.CreateModelVersionRequest) (*modelv1.ModelVersion, error) {
    // ... 现有校验 ...
    
    // 新增：tokenizer 校验
    if in.GetTokenizer() != nil {
        if err := validateTokenizerConfig(in.GetTokenizer()); err != nil {
            return nil, errors2.New(400, "INVALID_TOKENIZER", err.Error())
        }
    }
    
    // 如果提供了 storage_path，校验模型文件
    if in.GetStoragePath() != "" {
        validator := worker.ModelValidator{Storage: s.storage}
        result, err := validator.ValidateModel(ctx, p.TenantID, in.GetStoragePath())
        if err != nil {
            return nil, errors2.New(500, "VALIDATION_FAILED", err.Error())
        }
        
        if len(result.Errors) > 0 {
            return nil, errors2.New(400, "INVALID_MODEL", strings.Join(result.Errors, "; "))
        }
        
        // 记录 warnings 到日志
        for _, warning := range result.Warnings {
            log.Warn("model validation warning", "warning", warning)
        }
    }
    
    // ... 创建版本 ...
}
```

---

## 实施步骤

### 阶段 1：最小化增强（1-2 天）

**目标**：增加基本校验，不修改 proto

1. 在 `ImportExecutor` 中增加文件存在性检查
2. 检查必需文件：`config.json`, `tokenizer.json`
3. 如果缺失，标记 warning 但不阻止导入

**代码变更**：
```go
// internal/worker/import_executor.go

func (e ImportExecutor) executeManifest(...) error {
    // ... 现有逻辑 ...
    
    // 新增：检查必需文件
    hasConfig := false
    hasTokenizer := false
    
    for _, file := range files {
        if file.Path == "config.json" {
            hasConfig = true
        }
        if strings.Contains(file.Path, "tokenizer") {
            hasTokenizer = true
        }
    }
    
    if !hasConfig {
        e.log(ctx, "warning: config.json not found", t)
    }
    if !hasTokenizer {
        e.log(ctx, "warning: tokenizer files not found", t)
    }
    
    // ... 继续导入 ...
}
```

---

### 阶段 2：扩展 API（1 周）

**目标**：新增 tokenizer 字段

1. 修改 `api/model/v1/model.proto`
2. 重新生成 protobuf 代码
3. 更新数据库 schema（新增 `tokenizer_config` jsonb 列）
4. 实现自动提取逻辑

**Migration**：
```sql
-- 新增列存储 tokenizer 配置
ALTER TABLE model_versions 
ADD COLUMN tokenizer_config jsonb;

-- 新增列存储模型元数据
ALTER TABLE model_versions 
ADD COLUMN model_metadata jsonb;
```

---

### 阶段 3：与 Inference Service 集成（1 周）

**目标**：推理服务使用 tokenizer 配置

1. `ani-inference-service` 调用 `GetModelVersion` 时获取 tokenizer 配置
2. 创建 InferenceService 时传递 tokenizer 参数
3. 验证端到端流程

**示例**：
```go
// ani-inference-service 中

func (s *InferenceService) CreateInferenceService(ctx context.Context, in *Request) {
    // 1. 获取模型版本（包含 tokenizer 配置）
    modelResp, err := s.modelClient.GetModelVersion(ctx, &model_pb.GetModelVersionRequest{
        ModelVersionId: in.GetModelVersionId(),
    })
    
    // 2. 构造推理容器参数
    args := []string{
        "--model", modelResp.Version.StoragePath,
    }
    
    // 3. 添加 tokenizer 配置
    if tokenizerCfg := modelResp.Version.Tokenizer; tokenizerCfg != nil {
        if tokenizerCfg.Path != "" {
            args = append(args, "--tokenizer", tokenizerCfg.Path)
        }
        if tokenizerCfg.TrustRemoteCode {
            args = append(args, "--trust-remote-code")
        }
    }
    
    // 4. 创建 KServe InferenceService
    // ...
}
```

---

## 常见场景

### 场景 1：标准 Hugging Face 模型

```bash
# 模型文件结构
model/
├── config.json              # ✅ 必需
├── tokenizer.json           # ✅ 必需
├── tokenizer_config.json    # ✅ 推荐
├── special_tokens_map.json  # 可选
├── model.safetensors        # 模型权重
└── generation_config.json   # 可选
```

**自动配置**：
```json
{
  "tokenizer": {
    "type": "auto",
    "path": ".",
    "trust_remote_code": false
  }
}
```

---

### 场景 2：量化模型（GGUF）

```bash
model/
├── qwen-2.5-0.5b-q4_k_m.gguf  # 单文件模型
└── tokenizer.json             # 需要单独提供
```

**问题**：GGUF 文件不包含 tokenizer，需要额外上传

**解决方案**：
1. 导入时从原始模型仓库复制 tokenizer 文件
2. 或者在 `ImportModel` 时指定 `tokenizer_repo_id`

```protobuf
message ImportModelRequest {
  string repo_id = 3;           // "Qwen/Qwen2.5-0.5B-GGUF"
  string tokenizer_repo_id = 9; // "Qwen/Qwen2.5-0.5B-Instruct" (原始模型)
}
```

---

### 场景 3：自定义 Tokenizer

```python
# 训练时使用自定义 tokenizer
from transformers import AutoTokenizer

tokenizer = AutoTokenizer.from_pretrained("custom-tokenizer")
tokenizer.add_special_tokens({"additional_special_tokens": ["<|im_start|>", "<|im_end|>"]})
model.resize_token_embeddings(len(tokenizer))
```

**配置**：
```json
{
  "tokenizer": {
    "type": "custom",
    "path": ".",
    "trust_remote_code": true,
    "special_tokens": {
      "bos": "<|im_start|>",
      "eos": "<|im_end|>"
    }
  }
}
```

---

## 风险与考虑

### 1. 向后兼容性

**问题**：已有模型没有 tokenizer 配置

**解决方案**：
- 新字段设为可选
- 旧模型使用默认配置（`type: "auto"`）
- Inference Service 兼容两种模式

### 2. 验证性能

**问题**：下载并解压大模型验证很慢（70B 模型需要几分钟）

**解决方案**：
- 只下载前 100MB 检查必需文件
- 或者只验证 tar 归档的文件列表（不解压）

### 3. 多文件模型

**问题**：大模型拆分为多个文件（`model-00001-of-00003.safetensors`）

**当前支持**：
- ✅ 导入时自动打包为 tar.gz
- ✅ 推理引擎自动加载多分片模型

**无需额外处理**

---

## 参考资料

- [Hugging Face Tokenizer 文档](https://huggingface.co/docs/tokenizers/)
- [vLLM Tokenizer 配置](https://docs.vllm.ai/en/latest/models/engine_args.html#tokenizer)
- [Transformers Config 格式](https://huggingface.co/docs/transformers/main_classes/configuration)

---

## 下一步

1. **决定方案**：A（通过 startup_args）vs B（新增字段）
2. **实施阶段 1**：增加基本文件检查（不修改 proto）
3. **验证**：导入一个 Qwen 模型，检查日志中的 warnings
