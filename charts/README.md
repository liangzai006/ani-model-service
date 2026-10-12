# ANI Model Helm Chart

本目录只发布 Model 业务 Chart：`charts/ani-model`。它安装到已经存在的
`ani-system` 命名空间。Chart 不创建 Namespace，也不安装 PostgreSQL、S3 或
Inference。

| Chart | 管理的资源 | 不管理的资源 |
| --- | --- | --- |
| `ani-model` | 数据库 migration Job、Model Deployment、admin/gRPC Service、导入控制器、`ani-model-service` ServiceAccount 和 namespace 级 RBAC | Namespace、PostgreSQL、S3/MinIO、Storage 服务、KServe/LWS/Higress、Inference |

配置只有 `charts/ani-model/values.yaml`。`helm upgrade --install` 会先跑
migration Job，等它成功，再部署 Model。已执行的 SQL 记在
`public.schema_migrations`，之后会跳过。Inference 地址默认为空，Model 不依赖它启动。

安装步骤见 [Model 安装说明](../docs/deploy/helm/ani-model.md)。

```bash
export KUBE_CONTEXT="${KUBE_CONTEXT:?填写新集群 context}"
export S3_ENDPOINT="${S3_ENDPOINT:?填写新环境 S3 endpoint}"
test "$(kubectl config current-context)" = "$KUBE_CONTEXT" || {
  echo "当前 kubeconfig context 不是目标新集群" >&2
  exit 1
}
helm upgrade --install ani-model charts/ani-model \
  --namespace ani-system \
  --set-string storage.direct.endpoint="$S3_ENDPOINT" \
  --wait --timeout 15m
```
