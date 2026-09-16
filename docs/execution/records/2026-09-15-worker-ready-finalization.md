# Worker 版本 ready 收敛

导入 worker 增加可选 `VersionFinalizer`。当任务带有 `model_version_id` 且执行器成功时，worker 先调用带 tenant 的 `MarkReady`（由 PostgreSQL adapter 执行制品完整性 CAS），只有 ready 更新成功才确认任务 completed；ready 更新失败会走任务 retry，避免任务已完成但版本仍不可部署。未绑定版本的远程导入任务继续只完成任务本身，等待后续元数据绑定流程。

新增 worker 单元测试覆盖成功顺序和 ready 失败重试。开发机 `make verify` 全部通过。
