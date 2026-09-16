# ModelVersion ready 状态转换

新增 `MarkModelVersionReady` 和 `MarkModelVersionError` sqlc 更新。ready 转换只允许当前版本处于 `pending/importing`，并且同租户下存在非空引用、SHA256 与版本摘要完全匹配的制品；更新使用 tenant、version ID 和状态条件，旧或跨租户结果不会覆盖当前状态。

开发机 Docker PostgreSQL 已验证：匹配制品可进入 ready；缺少制品的版本被拒绝；ready 查询返回制品引用和摘要。`make verify` 全部通过。
