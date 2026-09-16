# 写入路径审计接入

## 变更

新增 `audit.Store` 端口，并在 Model、ModelVersion、ImportModel 成功受理后写入审计事件。事件来源于可信 Principal，包含 tenant、actor、workload、request_id、action、before/after 状态；导入任务额外记录 task_id。审计失败会返回 `AUDIT_UNAVAILABLE`，避免向调用方报告未记录的成功操作。

PostgreSQL 继续通过 `AuditStore` 和 sqlc 写入 `public.audit_events`，不引入跨服务表或事务。跨资源原子事务仍需在后续 pgx transaction composition slice 中完成。

## 验证

```text
make verify
PASS
```
