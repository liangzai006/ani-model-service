# Shared PostgreSQL development decision

用户确认开发阶段 Model 与 Inference 暂时共用同一个 PostgreSQL 数据库和访问账号权限。

- Model 继续使用`public` schema。
- Inference 继续使用自己的 `public.inference_*` 表。
- 不共享写表、不使用跨服务事务、不增加跨服务数据库外键。
- Model 后续连接池可沿用 Inference 的 `ANI_DATABASE_DSN` 环境变量，避免新增无必要的 typed config。
- 生产部署再拆分数据库账号和 schema 权限；当前凭据隔离状态标记为后续加固项。
