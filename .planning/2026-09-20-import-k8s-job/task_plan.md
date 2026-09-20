# 独立 Kubernetes 模型导入 Job

目标：ImportModel 提交持久任务后，创建独立 Kubernetes Import Job；Job 使用可恢复的大模型下载器（优先 ModelScope CLI），下载到大容量工作卷并将制品写入 Storage，Model 服务根据 Job 结果完成 artifact/ready。不能把进程内 worker 误称为 Kubernetes Job。

## 阶段
- [x] 盘点现有导入任务、Kubernetes 依赖、Storage 和状态模型边界
- [x] 设计并实现 Job 提交/状态观察端口与 Kubernetes adapter
- [x] 实现 Import Job 镜像/脚本契约（ModelScope CLI、Secret、PVC、校验、上传）
- [in_progress] 接入任务幂等、重试、完成/失败 CAS 和清理
- [in_progress] 补充测试、部署文档和验证结果

## 关键决策
- PostgreSQL `model_import_tasks` 仍是业务权威；Kubernetes Job 是执行投影。
- 不在 Model Go Pod 中 exec Python/CLI。
- 当前先保持 Inference 已有的 `.tar` artifact 合约；Job 以流式 tar 上传，不设 512 MiB 代码上限。
- 对象前缀 + manifest 仍是后续几百 GiB 模型的优化方向，不能把当前 tar 方案描述为分片制品。
- CLI token 只通过 Kubernetes Secret 注入，不进入请求、日志或 Job 参数。

## 错误记录
暂无。
