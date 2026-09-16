# 导入任务幂等重放

## 变更

新增 `public.model_import_tasks.request_fingerprint` 和对应 sqlc 查询。`WorkStore.Create` 先按 `(tenant_id, idempotency_key)` 查询：请求字段指纹一致时返回原任务（包括原 task ID），指纹不一致时返回冲突；并处理并发插入竞争后的重读。

指纹只用于幂等重放，不作为身份凭证。所有查询继续显式限定 `tenant_id`。

## 开发机验证

```text
MODEL_DATABASE_URL=postgres://.../recycling?sslmode=disable \
go test ./internal/data/postgres -run 'TestWorkStore(RemoteImportWithoutModelBinding|LeaseRecovery)Postgres' -count=1
ok

make verify
PASS
```

相同请求重放返回原 task，不同 payload 使用同一 key 被拒绝。CreateModel/CreateModelVersion 的幂等事务仍需接入各自写 use case。
