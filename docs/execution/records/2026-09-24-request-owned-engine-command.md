# 请求负责推理引擎启动配置

日期：2026-09-24

本次调整冻结了 Model 与 Inference 的启动命令边界：

- Model 的 `ModelVersion.engine_type`、`startup_command`、`startup_args` 只为兼容旧契约保留，Model client 查询 ready 版本时只读取制品引用、摘要和大小。
- Inference 的创建请求必须提供 `engine.type`、`engine.image` 和非空 `engine.command`；`engine.args` 也只来自同一个请求。
- Inference 不从 ModelVersion 回填引擎类型、镜像或命令，也不生成 `--served-model-name`、模型路径、端口等启动参数。请求中的命令数据会被保存到目标 runtime spec，再由 Deployment/KServe 适配器下发。
- 导入路径只有 Kubernetes Job：不再读取 `ANI_IMPORT_EXECUTION_MODE`，Job 使用镜像自身的 entrypoint 和 Model Deployment 的 `ani-model-service` ServiceAccount。
- PVC 容量不再接受 `ANI_IMPORT_STORAGE_SIZE`；worker 必须读取 provider manifest，按模型文件总大小增加 headroom 后计算容量，无法确定大小时拒绝创建 Job。

验证：Model 相关测试和 Inference 的 model adapter、service、server 测试通过；完整验证命令见本次执行记录后的测试结果。
