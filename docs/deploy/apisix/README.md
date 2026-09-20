# APISIX vLLM HTTP gateway

This deployment installs Apache APISIX and its Ingress Controller in the
`ingress-apisix` namespace. It runs beside the retired Envoy Gateway; it does
not delete or modify the existing Envoy `GatewayClass/ani-aigw` or its
namespaces.

The first slice exposes plain HTTP through a NodePort and routes an HTTPRoute to
one vLLM Service. Authentication, rate limiting, HTTPS, gRPC routes, IAM and
Quota remain deferred. The production Publication adapter now lives in the
Inference repository and creates or withdraws these HTTPRoutes through the
APISIX Gateway API controller.

## Install

The chart uses APISIX 3.18.0 through Helm chart 2.17.0 and reuses the Gateway
API CRDs already installed in the cluster. The admin key Secret must exist
before the release is installed:

```bash
kubectl create namespace ingress-apisix --dry-run=client -o yaml | kubectl apply -f -
kubectl -n ingress-apisix create secret generic ani-apisix-admin \
  --from-literal=admin="$(openssl rand -hex 24)" \
  --from-literal=viewer="$(openssl rand -hex 24)" \
  --dry-run=client -o yaml | kubectl apply -f -
helm repo add apisix https://apache.github.io/apisix-helm-chart
helm repo update apisix
helm upgrade --install ani-apisix apisix/apisix --version 2.17.0 \
  --namespace ingress-apisix --create-namespace \
  --values docs/deploy/apisix/values.yaml --wait --timeout 10m
kubectl apply -f docs/deploy/apisix/gateway.yaml
```

The APISIX proxy is available on NodePort `30090`. The Admin API remains
ClusterIP-only. Use a Kubernetes node InternalIP to reach the proxy.

## Verify the Gateway

```bash
kubectl -n ingress-apisix rollout status deployment/ani-apisix --timeout=5m
kubectl -n ingress-apisix get deployment,svc,gatewayproxy
kubectl get gatewayclass ani-apisix -o yaml
kubectl -n ingress-apisix get gateway ani-apisix -o yaml
```

The GatewayClass must have `Accepted=True`; the Gateway must have
`Accepted=True` and `Programmed=True`.

## vLLM smoke route

Apply `smoke-route.yaml` only while the recorded vLLM fixture service exists:

```bash
kubectl apply -f docs/deploy/apisix/smoke-route.yaml
APISIX_NODE_IP="$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')" \
  docs/deploy/apisix/verify-vllm.sh
```

The check sends `GET /v1/models` directly to the APISIX NodePort and expects a
JSON response from the vLLM Service. The route is a catch-all path route, so no
`Host` header is required. When DNS is configured later, use the DNS name in
the URL; it should resolve to the same APISIX node address.

## Remove the smoke route or release

```bash
kubectl delete -f docs/deploy/apisix/smoke-route.yaml
kubectl delete -f docs/deploy/apisix/gateway.yaml
helm uninstall ani-apisix --namespace ingress-apisix
kubectl delete namespace ingress-apisix
```

Removing this release is independent of the existing Envoy Gateway resources.

## Dashboard

APISIX 3.18 includes the embedded Dashboard at `/ui/`. The Helm release keeps
the original Admin Service as ClusterIP and exposes the UI through a separate
administrative NodePort:

```bash
kubectl apply -f docs/deploy/apisix/dashboard-service.yaml
APISIX_NODE_IP="$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')"
echo "http://${APISIX_NODE_IP}:30092/ui/"
```

The same endpoint exposes the Admin API under `/apisix/admin/`; it requires the
Admin Key in the `ani-apisix-admin` Secret. Treat NodePort `30092` as an
administrative endpoint and restrict access with the cluster or node firewall.
The model traffic remains on NodePort `30090`.
