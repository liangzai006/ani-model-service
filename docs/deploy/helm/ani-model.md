# 安装 Model

Model 不依赖 KServe、LWS、Gateway 或 Higress。Model gRPC 始终校验客户端证书。

只允许部署到新环境的 `ani-system`。必须显式传入新集群 context，脚本不会使用当前 context。

## 安装前

Namespace `ani-system` 已经存在，并带有标签 `ani.liangzai006.io/environment=new-cluster`。
PostgreSQL 里已经建好 Model 用的数据库。当前新集群的基础设施单独部署在
`ani-infra`，PostgreSQL 通过 `postgres.ani-infra.svc.cluster.local` 提供服务，
S3 通过 `minio.ani-infra.svc.cluster.local:9000` 提供服务；这些资源不属于本 Chart。
对象存储二选一：

- 直接 S3：bucket 已存在，Secret `ani-model-database` 有 key `dsn`，Secret `ani-model-minio` 有 `ANI_MINIO_ACCESS_KEY` 和 `ANI_MINIO_SECRET_KEY`。
- Storage gRPC：只配 `storage.mode=grpc`、地址、`serverName` 和 CA Secret，不要同时配直接 S3。

公共 `ClusterIssuer/ani-internal` 必须已经存在并且 cert-manager 已就绪。Chart 默认创建
`Certificate/ani-model-server-tls`，申请结果 Secret 包含 `ca.crt`、`tls.crt`、`tls.key`，
服务端身份为 `ani-model.ani.internal`。如果平台已经创建这张 Secret，可以设置
`tls.certificate.create=false`，Chart 只读取它。

Governance 调用 Model 时，客户端证书必须由该 CA 签发，且只有 DNS SAN
`ani-governance`，不能附带其他 DNS、URI、IP 或 email SAN，也不能只用 CN。
Model 的 RPC 白名单和可信租户/actor 分支均使用这一精确身份；旧身份
`ani-governance.ani.internal` 不再授权。上线本变更时须同步 Governance 的客户端
证书与配置。Inference 调用 Model 的现有登记身份仍为 `ani-inference.ani.internal`。

如果同时部署 Inference，请先确认平台基础设施已经安装并就绪，包括 cert-manager、
`ClusterIssuer/ani-internal` 和其他公共依赖，再执行这里的 Model 安装命令。Model
Chart 本身不会安装共享的 cert-manager、创建 `ClusterIssuer` 或接管已有资源，也不会触碰旧集群。

`/healthz` 和 `/readyz` 仍是普通 HTTP，给 kubelet 探针用，不要求客户端证书。

Chart 不创建 Namespace、数据库、bucket 或 Secret。地址、镜像和开关都在
`charts/ani-model/values.yaml`。默认 S3 地址为空，必须显式设置目标环境地址。

## 安装

```bash
export KUBE_CONTEXT="${KUBE_CONTEXT:?填写新集群 context}"
export S3_ENDPOINT="${S3_ENDPOINT:?填写新环境 S3 endpoint}"
test "$(kubectl config current-context)" = "$KUBE_CONTEXT" || {
  echo "当前 kubeconfig context 不是目标新集群" >&2
  exit 1
}

kubectl -n ani-system \
  get namespace ani-system -o 'jsonpath={.metadata.labels.ani\.liangzai006\.io/environment}'

helm upgrade --install ani-model charts/ani-model \
  --namespace ani-system \
  --set-string storage.direct.endpoint="$S3_ENDPOINT" \
  --wait --timeout 15m
```

要改某一项时直接改 `values.yaml`，或者临时覆盖：

```bash
helm upgrade --install ani-model charts/ani-model \
  --namespace ani-system \
  --set-string storage.direct.endpoint="$S3_ENDPOINT" \
  --set-string storage.direct.bucket=ani-models \
  --wait --timeout 15m
```

这一条命令的顺序是固定的：Helm 先启动 migration Job，等到 Job 成功，再部署 Model Deployment。Job 失败时安装停住，不会去更新模型服务。

Job 按 `charts/ani-model/migrations/*.sql` 的文件名顺序执行，并记到 `public.schema_migrations`。
迁移和账本写入在同一事务中完成，已经记过的文件会跳过。已有数据库如果没有可验证的
迁移账本会停止并提示人工确认，不会根据单个字段把全部迁移伪标记为已执行。

## 看结果

```bash
kubectl -n ani-system get jobs,deploy,pods
kubectl -n ani-system \
  rollout status deployment/ani-model-service --timeout=15m
kubectl -n ani-system port-forward svc/ani-model-admin 19091:19091
```

另一个终端：

```bash
curl -fsS http://127.0.0.1:19091/healthz
curl -fsS http://127.0.0.1:19091/readyz
```

`/healthz` 表示进程活着。`/readyz` 只检查 PostgreSQL、Storage 和导入 worker。

`inference.grpcAddress` 默认为空。只有在 Inference 提供对应 TLS 服务端时才设置它，并同时配置
`inference.serverName` 和 `inference.tls.existingSecret`（Secret 必须包含 `ca.crt`、`tls.crt`、`tls.key`）。
只填地址，进程会退出。
