# Higress Gateway Replacement Design

## Scope

The new cluster will use Higress as the only public inference gateway. The
existing environment and its resources are out of scope and must not be
changed. APISIX manifests remain historical records only; the new deployment
will not depend on them.

The external inference contract remains a fixed `POST /v1/completions` (and
the existing `/v1` endpoint URL returned by the control plane). Clients keep
sending `{"model":"<served-model-name>"}` in the JSON body.

## Architecture

Higress is installed by Helm with Gateway API support and a dedicated
`GatewayClass`/`Gateway` in `higress-system`. Its gateway Service is exposed
through the new cluster's NodePort. ANI continues to publish standard
Gateway API `HTTPRoute` objects, so the business publication port and durable
operation flow do not change.

Higress's `model-router` Wasm plugin reads the JSON `model` field on
`/completions` requests and writes a pinned `x-higress-llm-model` header.
Each published route matches that exact header and `/v1/completions`, then
forwards to the generation-specific model endpoint Service.

The Higress deployment enables its Prometheus-compatible gateway metrics and
AI statistics. Request-level rate limiting is enabled without IAM or Redis;
per-user identity and token quota remain deferred. Usage reporting is emitted
as asynchronous gateway log/metric data and is not written synchronously to
the control-plane PostgreSQL audit table.

## Code changes

- Rename the gateway publisher from APISIX-specific configuration to generic
  Higress configuration while preserving `publication.Port` and
  `EndpointResolver`.
- Add an exact `x-higress-llm-model` header match to each route.
- Read `ServedModelName` from `DesiredRuntime`; reject publication when it is
  empty, because a catch-all model route is unsafe.
- Replace `ANI_APISIX_*` deployment variables with `ANI_HIGRESS_*` variables.
- Keep route status confirmation based on Gateway API `Accepted` and
  `ResolvedRefs` conditions.

## Kubernetes deployment

- Add isolated Higress Helm values and installation documentation under
  `docs/deploy/higress/`.
- Use mirrored images under `docker.changqingyun.cn/mirror` and pin the
  Higress chart/plugin versions.
- Add the Higress `GatewayClass`, `Gateway`, model-router WasmPlugin, and
  monitoring/rate-limit configuration.
- Update the inference ServiceAccount Role only for resources used by the
  selected Gateway API route implementation.
- Verify two models on the same `/v1/completions` path, an unknown model
  response, streaming, metrics, and rate-limit responses before removing the
  new-cluster APISIX release.

## Non-goals

- No IAM, trusted identity, TLS, per-user quota, GPU scheduling, Notebook, or
  Kubeflow Dashboard work.
- No changes to the old environment.
- No model-specific public URL paths.
