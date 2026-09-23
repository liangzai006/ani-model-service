# Higress inference gateway for the new cluster

This directory is the active gateway definition for the cluster described by
`.clusterenv`. Higress replaced the new-cluster APISIX release after
validation. The old environment and its gateway resources are not touched.

Higress 2.2.4 is installed with Gateway API support. Its gateway is exposed
through NodePort `30090` (HTTPS `30443`) and uses the `higress` GatewayClass.
The built-in observability suite stores gateway metrics and logs in the new
cluster's CephFS-backed PVCs.

The model router plugin reads `model` from JSON requests whose path ends in
`/completions`, then adds `x-higress-llm-model`. ANI publishes an HTTPRoute for
each model with an exact match on that header and the fixed path
`/v1/completions`.

## Images

Mirror the exact chart images and Wasm plugins before installation. The source
images are:

```text
higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/higress:2.2.4
higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/pilot:2.2.4
higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:2.2.4
higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/console:2.2.4
higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/grafana:9.3.6
higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/prometheus:v2.40.7
higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/loki:2.9.4
higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/promtail:2.9.4
higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/model-router:2.0.2
higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/ai-statistics:2.0.2
higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/key-rate-limit:1.0.0
```

The deployment values and plugin manifest point to the corresponding
`docker.changqingyun.cn/mirror` locations.

## Install

Run these commands with the new-cluster kubeconfig from `.clusterenv`:

```bash
helm repo add higress.io https://higress.io/helm-charts
helm repo update higress.io
helm upgrade --install higress higress.io/higress \
  --version 2.2.4 \
  --namespace higress-system --create-namespace \
  --values docs/deploy/higress/new-cluster-values.yaml \
  --wait --timeout 15m
kubectl apply -f docs/deploy/higress/gateway.yaml
kubectl apply -f docs/deploy/higress/plugins.yaml
```

Then restart the new-cluster Inference Deployment with the `ANI_HIGRESS_*`
variables from `docs/deploy/inference/inference-control-plane.yaml`.

## Verification

Check the gateway and route status before sending traffic:

```bash
kubectl -n higress-system get pods,svc,gateway
kubectl get gatewayclass higress -o yaml
kubectl -n higress-system get gateway ani-higress -o yaml
kubectl -n higress-system get wasmplugin
kubectl -n ani-inference get httproute -o yaml
```

The acceptance test must use the same URL for every model:

```bash
curl -sS -X POST "http://192.168.102.68:30090/v1/completions" \
  -H 'content-type: application/json' \
  -d '{"model":"<served-model-name>","prompt":"hello","max_tokens":16}'
```

The repeatable smoke check is `verify-vllm.sh`; set `ANI_TEST_MODEL` and
optionally `ANI_HIGRESS_URL` before running it.

Verify two model names, an unknown model, a streaming request, gateway
metrics/AI statistics, and an HTTP 429 after the configured request limit is
exceeded. With IAM deferred, the current local limit is per served model:
`smollm2-135m` is configured for 10 requests/second. Add a `limit_keys` entry
when publishing another model; token quotas and identity-based limits remain
deferred.

## Rollback

Keep the Higress Helm values and manifests versioned. A gateway-only rollback
uses `helm rollback higress <revision> -n higress-system` followed by reapplying
`gateway.yaml` and `plugins.yaml`; the Inference Deployment remains on the
Higress variables. The legacy environment is independent of this operation.
