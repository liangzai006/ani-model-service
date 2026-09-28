# KServe adapter verification — 2026-09-23

## Scope and safety boundary

This verification used only the new cluster with API server `https://192.168.102.68:6443` and context `kubernetes-admin@kubekey`. No old-cluster context or legacy resource was used. The running control plane stays on the deployment provider; existing model services were not switched to KServe.

The change is intentionally uncommitted. Baseline commits remain `ani-inference-service` `f223725` and `ani-model-service` `9cfc05b`.

## What was implemented

ANI now has a KServe 0.15 `RawDeployment` runtime adapter. For a desired runtime it:

1. reads the complete model image, PVC, resources, ports, environment, and engine command from the persisted runtime spec;
2. creates or server-side-applies one `serving.kserve.io/v1beta1/InferenceService`;
3. lets KServe create the predictor Deployment and Service, then checks the Ready condition and the owned rollout;
4. publishes the existing fixed `/v1/completions` route to `<inferenceservice-name>-predictor:80` with the model header contract;
5. withdraws the route and foreground-deletes the InferenceService, allowing KServe-owned children to disappear.

ANI does not add engine flags or model defaults. KServe is selected per persisted generation through `runtime.provider`; omitted values remain `deployment` for compatibility, and an update without the field inherits the current provider. The running control plane remains deployment-backed while the new business API path is verified.

The new-cluster database has migrations `000013_inference_kserve_binding.sql`
and `000014_inference_runtime_provider.sql` applied, and the control plane runs:

`docker.changqingyun.cn/ani/inference-service:ani-inference-new-20260923-kserve-r4@sha256:dd29cf5a3f1252a0bd93547e7af35f6c9f4b1a4619581c7d498fe608cf87aa3a`

## Verification evidence

- `go test ./...`, `go vet ./...`, `go build ./...`, and `git diff --check` pass in `ani-inference-service`.
- `make verify` passes in `ani-model-service`.
- The opt-in live test `TestKServeLiveLifecycle` passed in 87.83s against the explicit new-cluster context. It covered KServe apply, predictor readiness, re-apply after KServe status changes, Higress model-header routing and a real completion, route withdrawal, foreground deletion, and removal of the child Deployment and Service.
- After cleanup, no KServe canary InferenceService, predictor Deployment,
  predictor Service, or temporary HTTPRoute remains. The only running
  inference workloads are the control plane and the pre-existing
  deployment-backed SmolLM2/Qwen services.
- Direct regression calls through new-cluster Higress returned HTTP 200 and non-empty completions for `smollm2-135m` and `qwen2.5-1.5b-instruct`.
- A separate real business API lifecycle then created a KServe-backed service
  through Model gRPC → PostgreSQL → Inference gRPC, returned a non-empty
  completion through Higress, and deleted the service. See [KServe gRPC lifecycle](2026-09-23-kserve-grpc-lifecycle.md).

## Remaining work

The adapter test still uses an in-memory binding store to isolate the
Kubernetes and publication adapters, while the separate business lifecycle
record covers the real Model gRPC → PostgreSQL operation runner. GPU and
larger-model KServe capacity tests are separate from this CPU smoke test.
