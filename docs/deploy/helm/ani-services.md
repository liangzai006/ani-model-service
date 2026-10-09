# ANI Model 与 Inference 新环境 Helm 部署文档

本文用于在一个新的 ANI 环境部署 Model 和 Inference。起点是：新的
Kubernetes 集群和 Ceph 已经准备好；ANI 的业务 Namespace、Secret、数据库
Schema、Model 和 Inference 资源都按本文从零创建。

本文不读取当前运行环境来决定部署内容，也不迁移或接管旧环境的资源。所有
命令都显式指定目标集群 context。PostgreSQL、S3/MinIO（需安装 MinIO Client `mc`）、Higress、KServe、LWS
和 Kubeflow 属于依赖，按各自的安装文档先准备；本文最后安装两个 ANI 业务
Chart。

## 1. 部署范围

| Chart | 创建的资源 | 不创建的资源 |
| --- | --- | --- |
| `charts/ani-model` | Model gRPC/admin Deployment、S3 兼容存储配置、导入 Job 控制器、ServiceAccount、namespaced RBAC | PostgreSQL、S3/MinIO、数据库迁移 Job、CRD、GPU 节点 |
| `charts/ani-inference` | Inference 控制面、gRPC/admin Service、KServe/LWS 和 Gateway API 权限、Higress 限流插件权限 | KServe/LWS CRD、Higress 安装、PostgreSQL、模型工作负载 |

Model 导入 Job 使用固定的 `ani-model-service` ServiceAccount。PVC 大小由模型
provider manifest 计算，Helm values 不提供手工大小参数。模型引擎的 image、
command、args、`--served-model-name` 和并行参数都由创建推理服务的业务请求
传入，Chart 不会拼接启动命令。

当前没有 GPU 的新环境保持 `accelerator.enabled=false`。GPU 节点和
Accelerator 服务准备好后，才单独启用 accelerator 配置。

## 2. 设置目标集群

把下面的 context 和网关节点地址替换成新环境的实际值。示例中的地址只表示
新环境入口，不代表旧环境。

```bash
export KUBE_CONTEXT="${KUBE_CONTEXT:-kubernetes-admin@kubekey}"
export GATEWAY_NODE_IP="${GATEWAY_NODE_IP:?请设置新环境网关节点 IP}"
export HIGRESS_PUBLIC_BASE_URL="http://${GATEWAY_NODE_IP}:30090"

kubectl --context "$KUBE_CONTEXT" config current-context
kubectl --context "$KUBE_CONTEXT" cluster-info
kubectl --context "$KUBE_CONTEXT" get nodes
helm version
```

`current-context` 必须与 `$KUBE_CONTEXT` 一致。如果 context 不匹配，停止执行，
不要把命令指向旧集群。

## 3. 准备基础依赖

### 3.1 创建基础数据库和 S3 凭据

凭据只通过本地环境变量传入，不写进 values、YAML 或 Git：

```bash
export POSTGRES_USER=ani
export POSTGRES_PASSWORD='替换为新环境数据库密码'
# Model 使用初始化数据库；Inference 使用下面单独创建的数据库。
export POSTGRES_DATABASE=ani_model
export MINIO_ROOT_USER=ani-storage
export MINIO_ROOT_PASSWORD='替换为新环境对象存储密码'

kubectl --context "$KUBE_CONTEXT" create namespace ani-foundation \
  --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -

kubectl --context "$KUBE_CONTEXT" -n ani-foundation create secret generic postgres-auth \
  --from-literal=username="$POSTGRES_USER" \
  --from-literal=password="$POSTGRES_PASSWORD" \
  --from-literal=database="$POSTGRES_DATABASE" \
  --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -

kubectl --context "$KUBE_CONTEXT" -n ani-foundation create secret generic minio-auth \
  --from-literal=username="$MINIO_ROOT_USER" \
  --from-literal=password="$MINIO_ROOT_PASSWORD" \
  --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -
```

### 3.2 部署新环境基础服务

在新的 `ani-foundation` Namespace 执行：

```bash
kubectl --context "$KUBE_CONTEXT" apply -f \
  docs/deploy/foundation/postgres-minio.yaml
kubectl --context "$KUBE_CONTEXT" -n ani-foundation \
  rollout status statefulset/postgres --timeout=10m
kubectl --context "$KUBE_CONTEXT" -n ani-foundation \
  rollout status statefulset/minio --timeout=10m

# 默认使用共享 bucket。Model 的签名上传接口要求 bucket 已存在。
# 开发机需要安装 MinIO Client (mc)，凭据只通过当前 shell 传入。
kubectl --context "$KUBE_CONTEXT" -n ani-foundation port-forward \
  svc/minio 19000:9000 >/tmp/ani-minio-port-forward.log 2>&1 &
MINIO_PF_PID=$!
trap 'kill "$MINIO_PF_PID" 2>/dev/null || true' EXIT
sleep 2
export MC_HOST_ANI="http://${MINIO_ROOT_USER}:${MINIO_ROOT_PASSWORD}@127.0.0.1:19000"
mc mb --ignore-existing ani/ani-models
unset MC_HOST_ANI
kill "$MINIO_PF_PID" 2>/dev/null || true
trap - EXIT

# 为 Inference 创建独立数据库。该命令只访问新环境的 PostgreSQL。
kubectl --context "$KUBE_CONTEXT" -n ani-foundation exec statefulset/postgres -- \
  sh -ec '
    export PGPASSWORD="$1"
    if ! psql -h 127.0.0.1 -U "$2" -d postgres -Atqc "select 1 from pg_database where datname='\''ani_inference'\''" | grep -q 1; then
      psql -v ON_ERROR_STOP=1 -h 127.0.0.1 -U "$2" -d postgres -c "CREATE DATABASE ani_inference OWNER $2"
    fi
  ' -- "$POSTGRES_PASSWORD" "$POSTGRES_USER"
```

Higress、Kubeflow v1.10、KServe 和 LeaderWorkerSet 按各自部署文档安装。先生成
本次新环境使用的网关 key（只保存在当前 shell），Higress 至少要有 `ani-higress` Gateway、Gateway API 和 ANI 使用的限流 WasmPlugin：

```bash
export ANI_HIGRESS_API_KEY="$(openssl rand -hex 32)"
helm repo add higress.io https://higress.io/helm-charts
helm repo update higress.io
helm upgrade --install higress higress.io/higress \
  --version 2.2.4 \
  --namespace higress-system --create-namespace \
  --values docs/deploy/higress/new-cluster-values.yaml \
  --kube-context "$KUBE_CONTEXT" \
  --wait --timeout 15m
kubectl --context "$KUBE_CONTEXT" apply -f docs/deploy/higress/gateway.yaml
kubectl --context "$KUBE_CONTEXT" apply -f docs/deploy/higress/plugins.yaml
kubectl --context "$KUBE_CONTEXT" apply -f <(sed "s|\${ANI_HIGRESS_API_KEY}|$ANI_HIGRESS_API_KEY|g" \
  docs/deploy/higress/key-auth.yaml.tmpl)
```

KServe/LWS 需要先完成 CRD 和控制器安装；本文不会用 Helm 创建这些集群级资源：

```bash
kubectl --context "$KUBE_CONTEXT" get crd \
  inferenceservices.serving.kserve.io \
  llminferenceservices.serving.kserve.io \
  leaderworkersets.leaderworkerset.x-k8s.io
```

## 4. 创建业务 Secret 和 Namespace

Model 和 Inference 使用不同的数据库 Secret。先从新环境数据库连接信息生成
两个 DSN，再创建应用 Secret：

```bash
export MODEL_DATABASE_DSN='postgresql://ani:替换密码@postgres.ani-foundation.svc.cluster.local:5432/ani_model'
export INFERENCE_DATABASE_DSN='postgresql://ani:替换密码@postgres.ani-foundation.svc.cluster.local:5432/ani_inference'

kubectl --context "$KUBE_CONTEXT" create namespace ani-model \
  --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -
kubectl --context "$KUBE_CONTEXT" label namespace ani-model \
  ani.liangzai006.io/environment=new-cluster --overwrite
kubectl --context "$KUBE_CONTEXT" create namespace ani-inference \
  --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -
kubectl --context "$KUBE_CONTEXT" label namespace ani-inference \
  ani.liangzai006.io/environment=new-cluster --overwrite

kubectl --context "$KUBE_CONTEXT" -n ani-model create secret generic ani-model-database \
  --from-literal=dsn="$MODEL_DATABASE_DSN" \
  --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -
kubectl --context "$KUBE_CONTEXT" -n ani-inference create secret generic inference-database \
  --from-literal=dsn="$INFERENCE_DATABASE_DSN" \
  --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -

kubectl --context "$KUBE_CONTEXT" -n ani-foundation create secret generic inference-postgres-auth \
  --from-literal=username="$POSTGRES_USER" \
  --from-literal=password="$POSTGRES_PASSWORD" \
  --from-literal=database=ani_inference \
  --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -

kubectl --context "$KUBE_CONTEXT" -n ani-model create secret generic ani-model-minio \
  --from-literal=ANI_MINIO_ACCESS_KEY="$MINIO_ROOT_USER" \
  --from-literal=ANI_MINIO_SECRET_KEY="$MINIO_ROOT_PASSWORD" \
  --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -
```

ModelScope/Hugging Face 通过 HTTPS 下载时，创建 CA ConfigMap：

```bash
kubectl --context "$KUBE_CONTEXT" -n ani-model create configmap ani-model-public-ca \
  --from-file=ca-certificates.crt=/etc/ssl/certs/ca-certificates.crt \
  --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -
```

如果镜像内置了所需信任根，可以在 Model values 中关闭 `caBundle.enabled`。

## 5. 执行数据库迁移

迁移必须在服务首次启动前完成。迁移 Job 不属于 Helm release，且只执行一次。
把仓库路径设置为当前部署机上的新环境仓库，不要使用开发机的固定路径。完整
Inference 迁移目录必须一次按序执行；其中的 KServe/LWS/GPU 字段是数据库结构，
即使当前没有 GPU 也必须执行。

```bash
export ANI_MODEL_REPO="$PWD"
export ANI_INFERENCE_REPO="/path/to/ani-inference-service"

kubectl --context "$KUBE_CONTEXT" -n ani-foundation create configmap \
  model-migrations-20260921 \
  --from-file="$ANI_MODEL_REPO/migrations" \
  --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -
kubectl --context "$KUBE_CONTEXT" apply -f \
  docs/deploy/foundation/model-migration-job.yaml
kubectl --context "$KUBE_CONTEXT" -n ani-foundation wait \
  --for=condition=complete job/model-migrations-20260921 --timeout=10m

kubectl --context "$KUBE_CONTEXT" -n ani-foundation create configmap \
  inference-migrations-20260922 \
  --from-file="$ANI_INFERENCE_REPO/migrations" \
  --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -
kubectl --context "$KUBE_CONTEXT" apply -f \
  docs/deploy/foundation/inference-migration-job.yaml
kubectl --context "$KUBE_CONTEXT" -n ani-foundation wait \
  --for=condition=complete job/inference-migrations-20260922 --timeout=10m
```

`inference-kserve-migration-job.yaml` 和 `inference-gpu-migration-job.yaml` 是
已有数据库的增量升级工具。全新环境执行完整目录后不要再次运行它们，否则会
重复执行 SQL；没有 GPU 也不能跳过完整目录中的数据库迁移。

## 6. 渲染并检查 Chart

使用新环境 values 文件。Inference 的公共 URL 通过变量传入，不从某个已部署
环境读取：

```bash
helm lint charts/ani-model
helm template ani-model charts/ani-model \
  --namespace ani-model \
  --kube-context "$KUBE_CONTEXT" \
  -f charts/ani-model/values-new-environment.yaml \
  > /tmp/ani-model-rendered.yaml

helm lint charts/ani-inference \
  --set higress.publicBaseURL="$HIGRESS_PUBLIC_BASE_URL"
helm template ani-inference charts/ani-inference \
  --namespace ani-inference \
  --kube-context "$KUBE_CONTEXT" \
  -f charts/ani-inference/values-new-environment.yaml \
  --set higress.publicBaseURL="$HIGRESS_PUBLIC_BASE_URL" \
  > /tmp/ani-inference-rendered.yaml

rg -n 'kind:|image:|secretKeyRef:|serviceAccountName:|leaderworkersets|llminferenceservices|ANI_HIGRESS_PUBLIC_BASE_URL' \
  /tmp/ani-model-rendered.yaml /tmp/ani-inference-rendered.yaml
```

确认镜像 digest、Secret 名称、固定 ServiceAccount、KServe/LWS 权限和 Higress
地址后再安装。

## 7. 安装 ANI Chart

### 7.1 Model

```bash
helm upgrade --install ani-model charts/ani-model \
  --namespace ani-model \
  --kube-context "$KUBE_CONTEXT" \
  -f charts/ani-model/values-new-environment.yaml \
  --wait --timeout 10m
kubectl --context "$KUBE_CONTEXT" -n ani-model \
  rollout status deployment/ani-model-service --timeout=10m
```

### 7.2 Inference

```bash
helm upgrade --install ani-inference charts/ani-inference \
  --namespace ani-inference \
  --kube-context "$KUBE_CONTEXT" \
  -f charts/ani-inference/values-new-environment.yaml \
  --set higress.publicBaseURL="$HIGRESS_PUBLIC_BASE_URL" \
  --wait --timeout 10m
kubectl --context "$KUBE_CONTEXT" -n ani-inference \
  rollout status deployment/ani-inference-service --timeout=10m
```

## 8. 新环境验证

```bash
kubectl --context "$KUBE_CONTEXT" -n ani-model get pods,svc,sa,role,rolebinding
kubectl --context "$KUBE_CONTEXT" -n ani-inference get pods,svc,sa,role,rolebinding
kubectl --context "$KUBE_CONTEXT" -n ani-inference get \
  inferenceservices.serving.kserve.io \
  llminferenceservices.serving.kserve.io \
  leaderworkersets.leaderworkerset.x-k8s.io \
  httproutes.gateway.networking.k8s.io
```

健康检查使用端口转发：

```bash
kubectl --context "$KUBE_CONTEXT" -n ani-model port-forward \
  svc/ani-model-admin 19091:19091 >/tmp/ani-model-pf.log 2>&1 &
MODEL_PF_PID=$!
curl -fsS http://127.0.0.1:19091/healthz
curl -fsS http://127.0.0.1:19091/readyz
kill "$MODEL_PF_PID"

kubectl --context "$KUBE_CONTEXT" -n ani-inference port-forward \
  svc/ani-inference-admin 19092:19091 >/tmp/ani-inference-pf.log 2>&1 &
INFERENCE_PF_PID=$!
curl -fsS http://127.0.0.1:19092/healthz
curl -fsS http://127.0.0.1:19092/readyz
kill "$INFERENCE_PF_PID"
```

模型导入和推理创建必须走 Model/Inference 业务 API。真实网关请求使用新环境
的 `x-api-key`：

```bash
export ANI_HIGRESS_API_KEY='从新环境的受保护凭据读取'
curl -fsS -X POST "$HIGRESS_PUBLIC_BASE_URL/v1/completions" \
  -H 'content-type: application/json' \
  -H "x-api-key: ${ANI_HIGRESS_API_KEY}" \
  -d '{"model":"<served-model-name>","prompt":"hello","max_tokens":16}'
```

只有返回真实生成结果，才算完成网关 → HTTPRoute → KServe/LWS → 模型运行时
链路验证。导入 Job/PVC 可按任务标签查看：

```bash
kubectl --context "$KUBE_CONTEXT" -n ani-model get jobs,pods,pvc \
  -l ani.liangzai006.io/task-id
```

## 9. 升级、回滚和清理

升级时继续使用同一个新环境 context 和 values 文件：

```bash
helm upgrade ani-model charts/ani-model \
  --namespace ani-model --kube-context "$KUBE_CONTEXT" \
  -f charts/ani-model/values-new-environment.yaml \
  --wait --timeout 10m
helm upgrade ani-inference charts/ani-inference \
  --namespace ani-inference --kube-context "$KUBE_CONTEXT" \
  -f charts/ani-inference/values-new-environment.yaml \
  --set higress.publicBaseURL="$HIGRESS_PUBLIC_BASE_URL" \
  --wait --timeout 10m
```

```bash
helm history ani-model --namespace ani-model --kube-context "$KUBE_CONTEXT"
helm history ani-inference --namespace ani-inference --kube-context "$KUBE_CONTEXT"
helm rollback ani-model <REVISION> --namespace ani-model \
  --kube-context "$KUBE_CONTEXT" --wait --timeout 10m
helm rollback ani-inference <REVISION> --namespace ani-inference \
  --kube-context "$KUBE_CONTEXT" --wait --timeout 10m
```

Helm 回滚不会回滚已经执行的 SQL。卸载前先通过业务 API 删除模型和推理服务，
让控制面清理 KServe、HTTPRoute、Job 和 PVC，再卸载两个 release：

```bash
helm uninstall ani-inference --namespace ani-inference --kube-context "$KUBE_CONTEXT"
helm uninstall ani-model --namespace ani-model --kube-context "$KUBE_CONTEXT"
```

不要使用 `kubectl delete all --all`，不要删除 `ani-foundation`、
`higress-system` 或任何旧环境资源。Chart 卸载不会删除外部数据库、S3 对象、
Secret、CRD 或 Namespace。

## 10. 故障排查

* **Pod Pending/ImagePullBackOff**：检查新环境镜像仓库、digest、节点资源和
  `imagePullSecrets`。
* **Model 未 Ready**：检查新环境 DSN、S3 endpoint/凭据和 CA ConfigMap。
* **导入 Job 创建失败**：检查固定 `ani-model-service` ServiceAccount、
  Role/RoleBinding、StorageClass 和导入镜像。
* **Inference 未 Ready**：检查 `inference-database`、Model gRPC Service、
  Higress URL 和 KServe/LWS CRD。
* **网关 401**：检查新环境 `x-api-key`，不要把 key 写入 values。
* **网关 404**：检查请求中的 `model` 是否等于 served model name，确认新环境
  HTTPRoute 已 Accepted。
* **GPU 失败**：先确认新环境节点上报 GPU，再检查 Accelerator mTLS；没有 GPU
  时只能验证 CPU 和控制面流程。
