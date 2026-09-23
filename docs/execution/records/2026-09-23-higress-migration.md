# 2026-09-23 Higress gateway replacement

## Scope

This change applies only to the new cluster in `.clusterenv`. The old
cluster and its gateway resources were not accessed or changed.

Higress 2.2.4 is now the only gateway for the new cluster. The new cluster
APISIX Helm release, namespace, GatewayClass and APISIX CRDs were removed after
traffic was validated through Higress. No APISIX resource remained in the new
cluster when cleanup completed.

## Gateway path

The public endpoint is:

```text
POST http://192.168.102.68:30090/v1/completions
```

The request body remains OpenAI-compatible and keeps the model selector in the
body:

```json
{"model":"smollm2-135m","prompt":"hello","max_tokens":4}
```

`model-router` extracts `model` and sets `x-higress-llm-model`. Inference
publishes one `HTTPRoute` per served model with an exact match on this Header
and the fixed `/v1/completions` path. The backend reference uses the generated
endpoint Service name (`<service-id>-endpoint`), so model names with dots do not
become Kubernetes Service names.

## Installed gateway components

- Higress Gateway API controller and Envoy gateway, exposed on NodePort 30090
  (HTTPS 30443).
- Prometheus, Grafana and Loki/Promtail using CephFS PVCs for gateway
  observability.
- `model-router` for body model extraction and multi-model routing.
- `ai-statistics` for OpenAI response usage, duration, first-token and stream
  statistics in gateway access logs and Prometheus-scraped Envoy metrics.
- `key-rate-limit` with a 10 requests/second local limit for the current
  `smollm2-135m` model. Add a `limit_keys` entry when another model is
  published. Its failure strategy is `FAIL_CLOSE`, so a broken limiter does
  not silently disable request protection. Per-user/token limits and IAM
  consumers remain deferred.

## Verification evidence

- All Higress pods were Ready, all observability PVCs were Bound, and
  `GatewayClass/higress` and `Gateway/ani-higress` reported Accepted/Programmed.
- The managed route reported controller
  `higress.io/gateway-controller` with Accepted and ResolvedRefs true.
- A real CPU completion through NodePort 30090 returned HTTP 200 and non-empty
  text.
- An unknown model and a request without `model` returned HTTP 404.
- Streaming completion returned SSE chunks and `[DONE]`.
- Temporarily lowering the model limit to 1 request/second produced one HTTP
  200 and three HTTP 429 responses under four concurrent requests; the
  configured limit was restored to 10 requests/second afterward.
- The gateway access log contained `ai_log` with model, input/output tokens,
  duration and stream/normal response type. Prometheus reported the Higress
  gateway target as `up` and exposed the WASM plugin statistics.
- The deployed Inference image is
  `docker.changqingyun.cn/ani/inference-service:ani-inference-new-20260923-higress-r4@sha256:d3c45d571b24a6f1b748ae98e4a0d08e5748c4ea929bf78165c4a1973aec1e18`.

## Inference runtime changes

The Inference service now always starts its Kubernetes runtime and creates the
ServiceAccount, Role, RoleBinding, model workloads, PVCs, Services and
Gateway API routes. `ANI_KUBERNETES_ENABLED` is no longer a runtime gate.
Quota reservation is opt-in in the executor and disabled for this validation
cluster; there is no fixed ten-unit quota provider. Trusted model identity,
TLS and IAM checks are not part of this validation scope.

Materialization rejects a ready ModelVersion whose persisted artifact size is
zero or negative instead of treating it as unlimited. This removes the old
512MiB assumption while keeping legacy rows from starting an unbounded
download; new larger models must carry their real `size_bytes` from Model
upload/confirm.

The verified path is create/materialize/publish/invoke. The previously recorded
Inference update lifecycle stale-generation retry remains a separate follow-up;
this gateway replacement does not claim that update path is fixed.

## Dual-model and node-placement verification

After the worker resources were increased, ordinary new-cluster workloads were
placed on `ani-k8s-cp` (`192.168.102.68`) where their templates allowed it. The
control-plane taint was removed for this shared validation cluster, and the
worker-2 node was cordoned during the move so existing Kubeflow, Istio,
Higress, Postgres, ft-sdk and authentication workloads could restart on the
control-plane. Ceph node-bound MON/OSD/exporter/CSI resources and DaemonSets
remain on their required nodes. Worker-2 is schedulable again for the pinned
Qwen runtime and future inference workloads.

An already-running `cpu-train-flow/cpu-smoke-01` Pod was left in place to
avoid interrupting an active training job. The `cpu-train-flow` visualization
server was moved to the control-plane; its GHCR application image is currently
`ImagePullBackOff` there, so the old ReplicaSet was scaled to zero and its
stale terminating Pods were removed. This image-pull issue is independent of
the inference gateway and does not affect the two model routes.

The Qwen 1.5B runtime is pinned to `ani-k8s-worker-2` and uses the materialized
3.1GB ModelVersion. With the ordinary workloads moved away, it reached Ready
without MemoryPressure. Both models were invoked through the same endpoint:

```text
POST http://192.168.102.68:30090/v1/completions
```

`model=qwen2.5-1.5b-instruct` returned HTTP 200 with generated text,
`model=smollm2-135m` returned HTTP 200 with generated text, and a Qwen
`stream=true` request returned SSE chunks and `[DONE]`. An unknown model was
rejected with HTTP 404. The Qwen route and the existing SmolLM route both
reported `Accepted=True` and `ResolvedRefs=True`, with ready EndpointSlices.

The Qwen rollout using an 8Gi memory limit was OOM-killed during request
verification after initially becoming ready. The validation Deployment was
raised to an 8Gi request and 12Gi limit on worker-2. The replacement became
ready with zero restarts; its cgroup reported approximately 8.83Gi memory use
and zero OOM kills. Repeating the eight-token completion request returned
HTTP 200 with eight generated tokens in 14.86 seconds. SmolLM also returned
HTTP 200 through the same gateway after this change. The terminated Qwen Pod
from the old, zero-replica ReplicaSet was removed.

Qwen uses the explicitly created validation Service
`qwen-1-5b-higress-live-endpoint` and HTTPRoute `ani-higress-qwen-live`.
The memory change is a validation-cluster Deployment override; the persisted
Inference spec and Model default engine command were not changed. These
checks establish real dual-model gateway invocation, not completion of the
deferred managed Update lifecycle.

## Engine argv override rollout

The Inference service was rebuilt from the engine-argv override changes and
pushed as:

```text
docker.changqingyun.cn/ani/inference-service:ani-inference-new-20260923-engine-argv-r1@sha256:46cf4e51049b974e4252b0c335be68ad6c3c2f046284bfd9bb4574473631eb53
```

Only the new-cluster `ani-inference-service` Deployment was updated. Its
control-plane Pod is now Ready on `ani-k8s-cp` (`192.168.102.68`), and the
deployment manifest pins the same image digest and node selector. The old
cluster was not accessed or changed.
