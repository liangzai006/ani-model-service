# API 模块结构对比分析

## 项目对比：ani-model-service vs ani-inference-service

生成日期：2026-10-20

---

## 📊 结构对比

| 维度 | ani-model-service (你的) | ani-inference-service | 评价 |
|------|-------------------------|----------------------|------|
| **API 独立模块** | ✅ `api/go.mod` | ✅ `api/go.mod` | 都正确 |
| **README 文档** | ✅ 详细 (2.1KB) | ❌ 无 | **你更好** |
| **Buf 配置** | ✅ `buf.yaml` | ❌ 无 | **你更好** |
| **API 子模块** | 3 个：`model/v1/`, `storage/v1/`, `inference/v1/` | 2 个：`inference/v1/`, `model/v1/` | **你更完整** |
| **业务逻辑位置** | ✅ 在 `internal/` | ❌ 混在 `api/` 中 | **你更规范** |
| **依赖其他服务 API** | ✅ 无（自己是源） | ❌ 复制了 `model.proto` | **你更正确** |

---

## ❌ ani-inference-service 的问题

### 问题 1：在 API 模块中包含业务逻辑

**错误做法**：
```
api/inference/v1/
├── business_payload.go       # 8.5KB 业务逻辑 ❌
├── business_validation.go    # 9.3KB 业务逻辑 ❌
├── validate.go               # 2.3KB 业务逻辑 ❌
├── inference.proto
├── inference.pb.go
└── inference_grpc.pb.go
```

**问题**：
- API 模块应该只包含 gRPC 协议定义
- 业务逻辑应该在服务的 `internal/` 中
- 客户端依赖这个 API 模块时会被迫拉取不需要的业务逻辑代码

### 问题 2：复制其他服务的 proto 文件

**错误做法**：
```
ani-inference-service/api/model/v1/model.proto
```

这是 Model Service 的 proto 副本，且包含错误的 `go_package`：
```protobuf
option go_package = "github.com/liangzai006/ani-inference-service/api/model/v1;modelv1";
```

**正确做法**：
```go
// ani-inference-service/go.mod
require github.com/liangzai006/ani-model-service/api v0.1.0

// ani-inference-service 代码中
import modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
```

**问题**：
- proto 文件不同步时会导致运行时不兼容
- 违反 DRY 原则
- 维护成本高（两处需要同步更新）

### 问题 3：无 README 文档

推理服务的 API 模块没有 README，下游服务不知道如何使用。

---

## ✅ ani-model-service 的优势

### 1. 结构清晰规范

```
api/
├── README.md          ✅ 完整文档
├── buf.yaml           ✅ Linting 配置
├── go.mod             ✅ 独立模块
├── go.sum
├── model/v1/          ✅ 只有 proto 定义
│   ├── model.proto
│   ├── model.pb.go
│   └── model_grpc.pb.go
├── storage/v1/        ✅ 只有 proto 定义
│   ├── storage.proto
│   ├── storage.pb.go
│   └── storage_grpc.pb.go
└── inference/v1/      ✅ 只有 proto 定义
    ├── model_reference.proto
    └── model_reference.pb.go
```

### 2. 职责分离清晰

- **`api/`** - 只包含 gRPC 协议定义
- **`internal/`** - 所有业务逻辑都在这里
- **客户端** - 只依赖 `api/`，不会拉取服务实现

### 3. 文档完善

README 包含：
- ✅ 模块路径
- ✅ 使用示例
- ✅ 目录结构
- ✅ 关键 API 列表
- ✅ 生成工具和命令
- ✅ 版本管理说明
- ✅ **明确警告不要复制 proto 文件**

### 4. Buf 配置

有 `buf.yaml` 用于 proto linting，确保 API 设计质量。

---

## 🎯 最佳实践总结

### ✅ 正确的 API 模块结构

1. **独立 go.mod**
   ```
   api/go.mod
   ```

2. **只包含协议定义**
   ```
   api/
   ├── service1/v1/*.proto
   ├── service1/v1/*.pb.go
   └── service1/v1/*_grpc.pb.go
   ```

3. **业务逻辑在服务内部**
   ```
   internal/
   ├── biz/
   ├── data/
   └── service/
   ```

4. **客户端通过 go.mod 依赖**
   ```go
   require github.com/org/service-name/api v1.2.3
   ```

5. **不复制其他服务的 proto**
   ```go
   // ❌ 错误：复制 proto 文件
   // api/other-service/v1/other.proto

   // ✅ 正确：通过 go.mod 依赖
   require github.com/org/other-service/api v1.0.0
   ```

### ❌ 反模式

1. ❌ 在 API 模块中包含业务逻辑
2. ❌ 在 API 模块中包含验证逻辑
3. ❌ 复制其他服务的 proto 文件
4. ❌ 没有 README 文档
5. ❌ 没有版本管理说明

---

## 📝 给推理服务的改进建议

### 建议 1：清理业务逻辑

**当前**：
```
api/inference/v1/
├── business_payload.go
├── business_validation.go
└── validate.go
```

**应该改为**：
```
# 移到服务内部
internal/biz/inference/
├── business_payload.go
├── business_validation.go
└── validate.go
```

### 建议 2：删除复制的 model.proto

**当前**：
```
api/model/v1/model.proto  # 复制的 ❌
```

**应该改为**：
```go
// go.mod
require github.com/liangzai006/ani-model-service/api v0.1.0

// 代码中
import modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
```

### 建议 3：添加 README

参考 `ani-model-service/api/README.md` 的结构。

---

## 🏆 结论

**ani-model-service 的 API 模块结构是标准的、正确的。**

推理服务的 API 模块需要重构：
1. 移除业务逻辑到 `internal/`
2. 删除复制的 proto 文件
3. 通过 go.mod 依赖其他服务的 API

你当前的实现可以作为团队的**最佳实践模板**。
