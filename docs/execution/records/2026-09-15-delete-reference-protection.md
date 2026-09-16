# DeleteModel Inference 引用保护

`DeleteModel` 现在要求配置版本化的 `InferenceReferenceChecker`。检查返回 active reference 时返回 `MODEL_IN_USE`/409；检查失败或 checker 未配置时返回 503，并拒绝按“无引用”处理；只有明确无引用才调用 ModelStore 的软删除。Model 不读取 Inference 数据库，正式实现仍需接入外部 gRPC/API 契约。

新增单元测试覆盖未配置检查器和存在引用两种拒绝路径。开发机 `make verify` 全部通过。
