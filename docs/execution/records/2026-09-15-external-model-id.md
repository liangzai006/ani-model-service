# External Model ID

日期：2026-09-15

- `public.models.id` 继续是内部 UUID 主键。
- 新增 `public.models.model_id` 作为租户范围唯一的对外模型标识，例如 `Qwen3-32B`。
- 历史行由 `name` 回填，避免已有数据失去外部标识。
- `model_versions.model_id` 仍是 UUID 外键，指向 `models.id`；它不改成字符串。
- `GetModelVersion` 已支持 `model_id + version` 解析到内部 UUID，旧
  `model_version_id` 查询保持兼容；typed query 显式限定 tenant、ready、非删除
  模型和完整制品摘要。
