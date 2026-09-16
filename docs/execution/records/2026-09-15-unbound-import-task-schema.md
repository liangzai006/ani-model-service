# Unbound remote import task schema

日期：2026-09-15

## 实现

- 远程导入先持久化来源参数，等待 provider 元数据解析后再绑定 Model/ModelVersion。
- `public.model_import_tasks.model_id` 改为可空；`model_version_id` 保持可空。
- `migrations/000005_allow_unbound_import_tasks_compat.sql` 兼容已经应用首个 migration 的开发数据库。
- ModelVersion 的 `model_id` 仍保持必填，避免产生没有所属模型的版本。

## 验证

- sqlc 生成代码仍与 schema 查询类型兼容。
- 不需要 PostgreSQL/监听端口的领域、worker、service 和 postgres adapter 测试通过。

## 未验证

- 开发机数据库尚未在本轮重新应用 `000002`；真实远程导入完成、provider 元数据绑定和进程重启恢复仍为 `not_verified`。
