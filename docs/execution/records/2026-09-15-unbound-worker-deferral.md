# Unbound import worker deferral

日期：2026-09-15

## 实现

- worker 领取任务后先检查 `model_version_id`。
- 未绑定任务不会调用 provider、不会上传对象、不会标记 completed。
- worker 使用 CAS retry 将任务恢复为 pending，并延后五分钟，等待可信绑定步骤补齐目标版本。
- 已绑定任务继续执行 provider 下载、Storage 校验、artifact 写入和 ready CAS。

## 验证

- 新增 `TestWorkerDefersUnboundTaskWithoutExecuting`，验证未绑定任务只 retry，不执行、不完成。
- `go test ./internal/worker` 通过。

## 未验证

- provider 元数据绑定流程尚未实现；未绑定任务目前会持续等待并产生 retry 指标。
