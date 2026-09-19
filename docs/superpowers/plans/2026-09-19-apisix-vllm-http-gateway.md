# APISIX vLLM HTTP Gateway Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Install Apache APISIX beside the retired Envoy Gateway and expose one real vLLM HTTP endpoint through APISIX using Kubernetes Gateway API.

**Architecture:** Keep the existing Envoy Gateway untouched while APISIX runs in `ingress-apisix`. The APISIX Ingress Controller watches a dedicated `GatewayClass`, translates an HTTP `Gateway`/`HTTPRoute` into APISIX configuration, and proxies `/v1/*` to the existing vLLM Kubernetes Service. The first slice intentionally has no authentication, rate limiting, IAM, Quota, or gRPC route; success means an HTTP request reaches vLLM.

**Tech Stack:** Apache APISIX 3.18.0, APISIX Helm chart 2.17.0, bundled APISIX Ingress Controller 2.2.0/1.3.0 chart dependency, Kubernetes Gateway API `GatewayClass`/`Gateway`/`HTTPRoute`, Helm 3.14.3.

## Global Constraints

- Install in the dedicated `ingress-apisix` namespace and do not delete or modify `envoy-ai-gateway-system`, `envoy-gateway-system`, `GatewayClass/ani-aigw`, or the existing Envoy `Gateway`.
- Reuse the already-installed Gateway API CRDs; set `ingress-controller.crds.gatewayAPI.enabled=false` so APISIX does not take ownership of Envoy-managed CRDs.
- Use HTTP `HTTPRoute` for vLLM OpenAI-compatible HTTP; do not create `GRPCRoute` in this slice.
- Expose APISIX HTTP with a non-conflicting NodePort `30090`; keep HTTPS disabled until TLS material and external DNS are supplied.
- Keep APISIX Admin API ClusterIP-only and use a Kubernetes Secret for the admin key; do not commit a plaintext admin key.
- Do not claim the complete Inference create → stop → restart → update → delete lifecycle is accepted; this plan validates only the gateway-to-vLLM HTTP path.
- Run chart rendering and Kubernetes validation before any cluster mutation; capture Gateway/HTTPRoute conditions and an actual HTTP response after installation.

---

### Task 1: Add reproducible APISIX installation and Gateway manifests

**Files:**
- Create: `docs/deploy/apisix/values.yaml`
- Create: `docs/deploy/apisix/gateway.yaml`
- Create: `docs/deploy/apisix/README.md`

**Interfaces:**
- `values.yaml` is consumed by `helm upgrade --install ani-apisix apisix/apisix --namespace ingress-apisix --values docs/deploy/apisix/values.yaml`.
- `gateway.yaml` produces `GatewayClass/ani-apisix`, `Gateway/ani-apisix`, and the route listener used by later HTTPRoute objects.
- The Gateway references `GatewayProxy/ani-apisix-config`, which is emitted by the Helm chart when `ingress-controller.gatewayProxy.createDefault=true`.

- [x] **Step 1: Write the pinned Helm values**

```yaml
# docs/deploy/apisix/values.yaml
service:
  type: NodePort
  http:
    enabled: true
    servicePort: 80
    containerPort: 9080
    nodePort: 30090
  tls:
    servicePort: 443

etcd:
  enabled: true
  replicaCount: 1
  persistence:
    enabled: true
    storageClass: ani-block
    size: 1Gi

apisix:
  admin:
    enabled: true
    type: ClusterIP
    credentials:
      secretName: ani-apisix-admin
      secretAdminKey: admin
      secretViewerKey: viewer

ingress-controller:
  enabled: true
  webhook:
    enabled: false
  config:
    controllerName: apisix.apache.org/ani-apisix-ingress-controller
    disableGatewayAPI: false
    listenerPortMatchMode: "off"
  gatewayProxy:
    createDefault: true
    provider:
      type: ControlPlane
      controlPlane:
        service:
          name: ani-apisix-admin
          port: 9180
        auth:
          type: AdminKey
          adminKey:
            valueFrom:
              secretKeyRef:
                name: ani-apisix-admin
                key: admin
  crds:
    gatewayAPI:
      enabled: false
  apisix:
    adminService:
      namespace: ingress-apisix
      name: ani-apisix-admin
      port: 9180
```

The install command creates the Secret before Helm so the APISIX pod and controller read the same key:

```bash
kubectl -n ingress-apisix create secret generic ani-apisix-admin \
  --from-literal=admin="$(openssl rand -hex 24)" \
  --from-literal=viewer="$(openssl rand -hex 24)" \
  --dry-run=client -o yaml | kubectl apply -f -
```

- [x] **Step 2: Define the APISIX Gateway API objects**

```yaml
# docs/deploy/apisix/gateway.yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: ani-apisix
spec:
  controllerName: apisix.apache.org/ani-apisix-ingress-controller
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: ani-apisix
  namespace: ingress-apisix
spec:
  gatewayClassName: ani-apisix
  infrastructure:
    parametersRef:
      group: apisix.apache.org
      kind: GatewayProxy
      name: ani-apisix-config
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
```

- [x] **Step 3: Document the one-command install and cleanup**

`docs/deploy/apisix/README.md` must contain the exact chart repo, release, namespace, values path, gateway apply command, NodePort `30090`, and rollback commands:

```bash
helm repo add apisix https://apache.github.io/apisix-helm-chart
helm repo update apisix
helm upgrade --install ani-apisix apisix/apisix --version 2.17.0 \
  --namespace ingress-apisix --create-namespace \
  --values docs/deploy/apisix/values.yaml
kubectl apply -f docs/deploy/apisix/gateway.yaml

helm uninstall ani-apisix --namespace ingress-apisix
kubectl delete gatewayclass ani-apisix
```

The README must state that the existing Envoy Gateway remains installed and that this slice has no auth, rate limiting, HTTPS, or gRPC route.

- [x] **Step 4: Validate static YAML before touching the cluster**

Run:

```bash
kubectl apply --dry-run=client -f docs/deploy/apisix/gateway.yaml
helm template ani-apisix apisix/apisix --version 2.17.0 \
  --namespace ingress-apisix --values docs/deploy/apisix/values.yaml \
  > /tmp/ani-apisix-rendered.yaml
rg -n 'kind: (GatewayProxy|GatewayClass|Deployment|Service)|nodePort: 30090|gatewayAPI|ani-apisix-ingress-controller' /tmp/ani-apisix-rendered.yaml
```

Expected: client-side validation succeeds; rendered output contains the APISIX HTTP NodePort `30090`, APISIX deployment/service, controller deployment, and `GatewayProxy`; no Gateway API CRD manifests are rendered.

---

### Task 2: Install APISIX without disturbing Envoy

**Files:**
- Modify: none in the service source tree.
- Cluster resources: namespace `ingress-apisix`, Helm release `ani-apisix`, the Secret from Task 1, and the Gateway objects from Task 1.

**Interfaces:**
- Consumes the values and manifests from Task 1.
- Produces a Ready APISIX data plane, APISIX Ingress Controller, `GatewayClass/ani-apisix`, and `Gateway/ingress-apisix/ani-apisix`.

- [x] **Step 1: Confirm the pre-install invariants**

```bash
kubectl get gatewayclass ani-aigw -o jsonpath='{.spec.controllerName}{"\n"}'
kubectl get ns envoy-ai-gateway-system envoy-gateway-system
kubectl get crd gateways.gateway.networking.k8s.io httproutes.gateway.networking.k8s.io
kubectl get svc -A -o json | jq -r '.items[] | .spec.ports[]? | select(.nodePort == 30090) | .nodePort'
```

Expected: Envoy class/controller and namespaces exist, Gateway API CRDs exist, and the final command prints no result.

- [x] **Step 2: Create/update the admin Secret and install the Helm release**

Run the Secret command from Task 1, then:

```bash
helm upgrade --install ani-apisix apisix/apisix --version 2.17.0 \
  --namespace ingress-apisix --create-namespace \
  --values docs/deploy/apisix/values.yaml --wait --timeout 10m
kubectl apply -f docs/deploy/apisix/gateway.yaml
```

- [x] **Step 3: Wait for APISIX and controller readiness**

```bash
kubectl -n ingress-apisix rollout status deployment/ani-apisix --timeout=5m
kubectl -n ingress-apisix rollout status deployment/ani-apisix-ingress-controller --timeout=5m
kubectl -n ingress-apisix get pods,svc,gatewayproxy
kubectl get gatewayclass ani-apisix -o yaml
kubectl -n ingress-apisix get gateway ani-apisix -o yaml
```

Expected: both deployments roll out; GatewayClass is `Accepted=True`; Gateway is `Accepted=True` and `Programmed=True`; APISIX Service exposes HTTP NodePort `30090`. If the chart names the controller deployment differently, discover it with `kubectl -n ingress-apisix get deployment` and use its actual name in the rollout check.

- [x] **Step 4: Verify Envoy is unchanged**

```bash
kubectl get gatewayclass ani-aigw -o jsonpath='{.spec.controllerName}{"\n"}'
kubectl -n envoy-ai-gateway-system get deployment,svc --no-headers
```

Expected: `ani-aigw` still points to `gateway.envoyproxy.io/gatewayclass-controller`, and its deployment/service remain present.

---

### Task 3: Route one real vLLM HTTP Service through APISIX and verify traffic

**Files:**
- Create: `docs/deploy/apisix/smoke-route.yaml`
- Create: `docs/deploy/apisix/verify-vllm.sh`

**Interfaces:**
- `smoke-route.yaml` is a concrete route for the current known vLLM fixture `smollm2-live-20260916-final-published` in namespace `ani-model-inference-e2e-retry-20260916`, on port `80`, with host `smollm2.vllm.test` and path prefix `/v1`.
- `verify-vllm.sh` accepts `APISIX_NODE_IP` and performs the route-condition and HTTP smoke checks against `http://${APISIX_NODE_IP}:30090/v1/models` with `Host: smollm2.vllm.test`.

- [x] **Step 1: Add the concrete HTTPRoute**

```yaml
# docs/deploy/apisix/smoke-route.yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: smollm2-vllm
  namespace: ani-model-inference-e2e-retry-20260916
spec:
  parentRefs:
    - name: ani-apisix
      namespace: ingress-apisix
      sectionName: http
  hostnames:
    - smollm2.vllm.test
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /v1
      backendRefs:
        - name: smollm2-live-20260916-final-published
          port: 80
```

- [x] **Step 2: Apply the route and inspect conditions**

```bash
kubectl apply -f docs/deploy/apisix/smoke-route.yaml
kubectl -n ani-model-inference-e2e-retry-20260916 get httproute smollm2-vllm -o yaml
kubectl -n ani-model-inference-e2e-retry-20260916 describe httproute smollm2-vllm
```

Expected: `Accepted=True` and `ResolvedRefs=True`; no cross-namespace `ReferenceGrant` is required because the backend Service and HTTPRoute share a namespace.

- [x] **Step 3: Verify an actual vLLM response through the APISIX NodePort**

```bash
APISIX_NODE_IP="$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')" \
  docs/deploy/apisix/verify-vllm.sh
```

The script must:

```bash
#!/usr/bin/env bash
set -euo pipefail
: "${APISIX_NODE_IP:?set APISIX_NODE_IP to a Kubernetes node address}"
route_status="$(kubectl -n ani-model-inference-e2e-retry-20260916 get httproute smollm2-vllm -o jsonpath='{range .status.parents[*].conditions[*]}{.type}={.status} {end}')"
grep -q 'Accepted=True' <<<"$route_status"
grep -q 'ResolvedRefs=True' <<<"$route_status"
response="$(curl --fail-with-body --silent --show-error --max-time 20 \
  -H 'Host: smollm2.vllm.test' \
  "http://${APISIX_NODE_IP}:30090/v1/models")"
grep -Eq '"data"|"object"|"model"' <<<"$response"
printf '%s\n' "$response"
```

Expected: the route conditions pass and curl returns a JSON vLLM model response through APISIX. A 404/503 is a failed slice and must be debugged using APISIX controller logs, APISIX pod logs, Service endpoints, and the route conditions before claiming success.

- [x] **Step 4: Document cleanup and the next integration boundary**

```bash
kubectl delete -f docs/deploy/apisix/smoke-route.yaml
kubectl delete -f docs/deploy/apisix/gateway.yaml
```

The README must explain that production `Publication` wiring still needs a versioned publish/withdraw contract carrying the runtime Service, hostname, and route identity; it is intentionally outside this first HTTP smoke slice and will be added after IAM/Quota contracts are available.

---

## Verification Checklist

- [x] `helm template` renders with no APISIX-managed Gateway API CRDs.
- [x] `GatewayClass/ani-apisix` is accepted by `apisix.apache.org/ani-apisix-ingress-controller`.
- [x] `Gateway/ingress-apisix/ani-apisix` is programmed and APISIX Service uses NodePort `30090`.
- [x] `HTTPRoute/smollm2-vllm` reports `Accepted=True` and `ResolvedRefs=True`.
- [x] `curl -H 'Host: smollm2.vllm.test' http://NODE_IP:30090/v1/models` returns JSON from vLLM.
- [x] Existing Envoy Gateway resources are still present and unchanged.
- [x] No claim is made that the complete Inference lifecycle or IAM/Quota/Publication production integration is complete.
