# Model 与 ModelVersion 幂等写入

## 变更

新增 `public.models` 和 `public.model_versions` 的 `idempotency_key`、`request_fingerprint` 字段及租户范围的部分唯一索引。ModelStore、VersionStore 在创建前按租户和幂等键查询：相同请求返回已存在记录，不同请求参数返回 Conflict；空幂等键保持兼容并不建立唯一约束。

gRPC 创建 handler 已把请求幂等键传入持久化层。请求指纹只用于重放一致性校验，不作为身份凭证。

## 开发机验证

```text
MODEL_DATABASE_URL=postgres://.../recycling?sslmode=disable \
go test ./internal/data/postgres -run TestModelAndVersionIdempotencyPostgres -count=1
ok

make verify
PASS
```
