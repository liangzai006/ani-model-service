# Higress Gateway Replacement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the new-cluster APISIX inference gateway with Higress while preserving the fixed `/v1/completions` contract and leaving the old environment untouched.

**Architecture:** Higress runs as the only new-cluster public gateway with Gateway API enabled. A pinned Higress model-router WasmPlugin converts the JSON `model` field into `x-higress-llm-model`; ANI publishes per-model HTTPRoutes matching that header and the fixed completion path. Higress metrics and request-level controls are deployed with the gateway, while IAM-backed quotas remain deferred.

**Tech Stack:** Go, controller-runtime, Kubernetes Gateway API v1, Higress Helm chart 2.2.4, Higress WasmPlugin CRD, Prometheus-compatible metrics.

## Global Constraints

- Modify only the new-cluster deployment and the current ANI inference repository; do not change legacy runtime resources or legacy APISIX values.
- Keep the external endpoint `POST /v1/completions` and the JSON `model` field.
- Do not add IAM, trusted identity, TLS, per-user quota, Notebook, Dashboard, or GPU work.
- Keep `publication.Port` and `publication.EndpointResolver` stable.
- Pin Higress and plugin image versions and mirror images under `docker.changqingyun.cn/mirror`.

---

### Task 1: Refactor the publication adapter for Higress model routes

**Files:**
- Modify: `/root/kubercon/ani-inference-service/internal/data/kubernetes/http_route_publisher.go`
- Modify: `/root/kubercon/ani-inference-service/internal/data/kubernetes/http_route_publisher_test.go`
- Modify: `/root/kubercon/ani-inference-service/cmd/ani-inference-service/main.go`
- Modify: `/root/kubercon/ani-inference-service/README.md`

**Interfaces:**
- Consumes: `DesiredRuntime.ServedModelName`, `DesiredRuntime.ServiceID`, existing `publication.Publication`.
- Produces: Higress-compatible `HTTPRoute` resources and the same `publication.Port`/`EndpointResolver` behavior.

- [ ] Rename APISIX-specific publisher comments, fields, and environment variables to Higress/gateway names while retaining the existing route status contract.
- [ ] Build each route with `PathPrefix(/v1/completions)` and an exact request-header match `x-higress-llm-model=<ServedModelName>`.
- [ ] Reject publication when `ServedModelName` is empty.
- [ ] Derive the backend Service name from the stable ServiceID (`<serviceID>-endpoint`) instead of the display/model name.
- [ ] Update unit tests for header/path/backend matching, empty served model rejection, publish/withdraw fencing, and endpoint URL construction.
- [ ] Run `go test ./internal/data/kubernetes -count=1` and `go test ./... -count=1`.

### Task 2: Add isolated Higress deployment and plugin manifests

**Files:**
- Create: `/root/kubercon/ani-model-service/docs/deploy/higress/new-cluster-values.yaml`
- Create: `/root/kubercon/ani-model-service/docs/deploy/higress/gateway.yaml`
- Create: `/root/kubercon/ani-model-service/docs/deploy/higress/plugins.yaml`
- Create: `/root/kubercon/ani-model-service/docs/deploy/higress/README.md`
- Modify: `/root/kubercon/ani-model-service/docs/deploy/inference/inference-control-plane.yaml`

**Interfaces:**
- Consumes: Higress Helm chart 2.2.4 and Gateway API v1.6 CRDs.
- Produces: `higress-system` gateway, NodePort, GatewayClass/Gateway, model-router WasmPlugin, AI statistics plugin, and request-level rate-limit configuration.

- [ ] Configure Higress Gateway API support, isolated namespace, mirrored images, `NodePort 30090`, and a pinned chart version.
- [ ] Configure gateway Prometheus annotations and the new-cluster observability storage class without changing Kubeflow's Istio gateway.
- [ ] Add a GatewayClass controller name `higress.io/gateway-controller` and Gateway `higress-system/ani-higress` with an HTTP listener allowing routes from `ani-inference`.
- [ ] Add a pinned `model-router` WasmPlugin scoped to the Higress gateway, explicitly setting `modelToHeader: x-higress-llm-model` and `/completions` suffix support.
- [ ] Add AI statistics configuration for OpenAI-compatible request/response usage and a local request-limit plugin configuration that does not require IAM or Redis.
- [ ] Change the inference Deployment environment variables and Role comments from `ANI_APISIX_*` to `ANI_HIGRESS_*`; keep Gateway API `HTTPRoute` RBAC.
- [ ] Document image mirroring, installation, verification, rollback, and the fact that old APISIX resources are out of scope.

### Task 3: Mirror images and migrate the new cluster

**Files:**
- Create: `/root/kubercon/ani-model-service/docs/deploy/higress/verify-vllm.sh`
- Create: `/root/kubercon/ani-model-service/docs/execution/records/2026-09-23-higress-migration.md`

**Interfaces:**
- Consumes: `.clusterenv`, Higress manifests, current model endpoint Services.
- Produces: A verified Higress NodePort and evidence for route, model selection, streaming, metrics, rate limits, and rollback.

- [ ] Mirror the pinned Higress, console/observability, and Wasm plugin images to `docker.changqingyun.cn/mirror` without overwriting existing tags.
- [ ] Install Higress on the new cluster and wait for controller, gateway, and observability workloads.
- [ ] Apply Gateway/Plugin manifests and restart only the new-cluster inference Deployment with Higress variables.
- [ ] Publish two model routes and verify both models through the same `/v1/completions` URL; verify unknown models do not reach an arbitrary backend.
- [ ] Verify streaming, Prometheus metrics, AI statistics, and HTTP 429 request limiting.
- [ ] Remove new-cluster APISIX only after all checks pass; do not access or modify the old cluster.
- [ ] Record exact commands, image digests, resource status, and any deferred AI-token/identity gaps.

### Task 4: Repository verification and cleanup

**Files:**
- Modify: `/root/kubercon/ani-model-service/docs/execution/status.md`
- Modify: `/root/kubercon/ani-model-service/docs/deploy/kubeflow-v1.10/README.md` only if it references the replaced new-cluster gateway.

- [ ] Remove new-cluster status statements that claim APISIX is the active inference gateway.
- [ ] Keep historical APISIX records intact and clearly mark them as superseded for the new cluster.
- [ ] Run `git diff --check`, service tests, manifest rendering, and final new-cluster read-only checks.
