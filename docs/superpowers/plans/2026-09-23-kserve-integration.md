# KServe Runtime Integration Implementation Plan

> **For agentic workers:** This plan is executed inline in the current workspace. Do not create a commit after implementation; the baseline was committed before this plan.

**Goal:** Add a KServe 0.15 RawDeployment backend for ANI inference while preserving Model ownership, full caller-provided engine argv, CephFS model materialization, and Higress `/v1/completions` publication.

**Architecture:** PostgreSQL and ANI remain the business authority. A runtime provider creates one `serving.kserve.io/v1beta1 InferenceService` per ANI generation; KServe owns its Deployment and predictor Service. Higress is the inference northbound/data-plane gateway and routes the fixed completion path to the KServe predictor Service. ani-governance is a sibling control-plane gateway for model and inference management APIs and does not proxy Higress.

**Tech Stack:** Go, controller-runtime, KServe v0.15 `InferenceService`, Kubernetes Gateway API, CephFS PVC, Higress.

## Global Constraints

- Do not touch the old cluster or legacy resources.
- Do not change KServe's shared default; every managed service explicitly selects `RawDeployment`.
- ANI must never add, remove, or rewrite engine startup arguments.
- KServe owns generated Deployment/Service objects; ANI owns only its InferenceService and durable bindings.
- Existing running ANI Deployment paths remain available until the KServe path is verified.
- No IAM, TLS, trusted identity, quota, Notebook, Trainer, or Pipeline work in this slice.

### Task 1: KServe object renderer and provider adapter — complete

**Files:**
- Create: `/root/kubercon/ani-inference-service/internal/data/kubernetes/kserve.go`
- Create: `/root/kubercon/ani-inference-service/internal/data/kubernetes/kserve_test.go`
- Modify: `/root/kubercon/ani-inference-service/internal/data/kubernetes/controller.go`

Implemented and unit-tested. The adapter renders and owns one KServe `InferenceService`, preserves the complete caller-provided command, mounts the materialized model PVC, and waits for the Ready condition plus the owned predictor Deployment rollout. KServe 0.15 copies the predictor Deployment observed generation into the InferenceService status, so readiness compares those two values and validates the desired container fields explicitly.

### Task 2: Publication backend resolution — complete

**Files:**
- Modify: `/root/kubercon/ani-inference-service/internal/data/kubernetes/http_route_publisher.go`
- Modify: corresponding publisher tests

Implemented and unit-tested. The KServe provider publishes to `<inferenceservice-name>-predictor:80`, while the deployment provider keeps the existing endpoint Service. The fixed `/v1/completions` contract and exact model-header routing remain unchanged.

### Task 3: Composition, persistence binding, and RBAC — implemented, migration applied

**Files:**
- Modify: `/root/kubercon/ani-inference-service/cmd/ani-inference-service/main.go`
- Modify: `/root/kubercon/ani-inference-service/internal/data/kubernetes/executor.go`
- Modify: `/root/kubercon/ani-inference-service/charts/ani-inference`

Added a persisted per-generation `runtime.provider` (`deployment` or `kserve`), a distinct `KServeInferenceService` binding kind, migrations `000013` and `000014`, KServe RBAC, and a runtime router. Omitted provider remains deployment for compatibility; updates inherit the current provider unless one is explicitly supplied. The running control plane remains deployment-backed so existing services are not migrated.

### Task 4: Verification — complete for adapter and business lifecycle

Focused tests, full `go test ./...`, `go vet ./...`, `go build ./...`, and `git diff --check` pass. An explicit new-cluster adapter live test passed in 87.83s: KServe apply, Ready, repeated apply after status writes, Higress model-header `/v1/completions`, publication withdrawal, foreground deletion, and dependent Deployment/Service removal. The canary resources were cleaned up. A separate real Model gRPC → PostgreSQL operation-runner lifecycle also passed with `runtime.provider: kserve`.

## Current boundary

The adapter and per-service provider decision passed a controlled business API
test. A new KServe-backed service was created through the real Model →
Inference gRPC and PostgreSQL lifecycle, served through Higress, and deleted;
all existing Deployment services remained untouched. GPU and larger-model
capacity are separate follow-up work. The current delete workflow removes
KServe/runtime and route objects but does not automatically garbage-collect
materialization PVC/Job/Secret objects; the verification cleaned its own test
objects with an exact service-id selector.
