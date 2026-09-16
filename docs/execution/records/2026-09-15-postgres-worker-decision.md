# Import task persistence decision

日期：2026-09-15

## 决策

Model 导入任务继续采用 PostgreSQL 持久任务表和独立 worker，不迁移为 Kubernetes CRD，也不引入 controller-runtime 作为任务主调度器。

## 原因

- Model、ModelVersion 和 ImportTask 都是 Model 业务数据，保持在同一个 PostgreSQL 本地事务边界内。
- `public` schema、sqlc、显式 `tenant_id` 和现有 lease/CAS fencing 设计保持不变。
- Model 不拥有 Kubernetes 资源，服务不应依赖 Kubernetes API 才能运行。
- PostgreSQL 任务表支持进程重启恢复、due scan、重试和多实例竞争处理。
- CRD 与 PostgreSQL 之间无法原子提交，会引入双写和状态同步问题。

## 后续扩展

如果未来需要 GitOps 或 `kubectl` 入口，可以增加独立 CRD adapter/controller 调用 Model gRPC；Model 核心业务数据和导入任务仍由 PostgreSQL 负责。
