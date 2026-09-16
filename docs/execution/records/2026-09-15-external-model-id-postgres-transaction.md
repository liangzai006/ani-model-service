# External model ID PostgreSQL transaction check

日期：2026-09-15

在本机 Docker `recycling-postgres` 的 `recycling` 数据库中，用一个事务临时增加
`public.models.model_id`、唯一索引和随机 tenant/model/version/artifact 数据，执行
ready 外部标识查询及另一 tenant 查询，结果分别为 `1` 和 `0`；事务最后回滚，未
留下数据或 schema 变更。

该验证确认 SQL 的 ready、非删除模型、完整 SHA256 制品和 tenant 条件组合正确。
持久 migration、Go adapter 绑定和真实导入任务仍需下一阶段验证，当前保持
`not_verified`。
