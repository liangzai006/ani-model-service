# Lease renewal fencing

日期：2026-09-15

## 实现

- worker 续租 goroutine 现在检查 PostgreSQL CAS 续租错误。
- 续租失败立即取消 provider/Storage 执行 context，旧 worker 不再继续上传或完成任务。
- 任务完成仍需原 owner 和 lease epoch 条件；续租失败结果进入 retry，由新 lease 接管。

## 验证

- 新增续租失败测试，验证执行被取消、任务 retry 且不会 completed。
- `go test ./internal/worker` 通过。

## 未验证

- 真实 PostgreSQL lease 过期与进程崩溃恢复仍为 `not_verified`。
