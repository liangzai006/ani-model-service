# APISIX vLLM HTTP gateway verification

Date: 2026-09-19

## Scope

Apache APISIX was installed beside the retired Envoy Gateway in namespace
`ingress-apisix`. The existing Gateway API CRDs remain owned by the existing
cluster installation; APISIX uses its own GatewayClass controller name:
`apisix.apache.org/ani-apisix-ingress-controller`.

This record validates only HTTP access to an existing vLLM Service. It does not
claim production Publication, IAM, Quota, authentication, rate limiting,
HTTPS, or the complete Inference lifecycle.

## Installed resources

- Helm release: `ani-apisix`, chart `apisix` `2.17.0`, APISIX `3.18.0`.
- APISIX Ingress Controller: `2.2.0`.
- etcd: one replica with a `1Gi` PVC on StorageClass `ani-block`.
- Gateway API: `GatewayClass/ani-apisix`, `Gateway/ingress-apisix/ani-apisix`.
- External HTTP: Service `ingress-apisix/ani-apisix-gateway`, NodePort `30090`.
- Existing Envoy Gateway was left in place.

## Verification evidence

```text
GatewayClass ani-apisix: Accepted=True Accepted
Gateway ani-apisix: Accepted=True Accepted; Programmed=True Programmed
HTTPRoute smollm2-vllm: Accepted=True Accepted; ResolvedRefs=True ResolvedRefs
```

The first request returned 404 because the APISIX route contained
`server_port == 80` while the APISIX container listens on `9080` behind the
NodePort. Setting the controller's `listenerPortMatchMode` to the quoted YAML
string `"off"` removed that condition. The final request was:

```text
GET http://<node-internal-ip>:30090/v1/models
Host: smollm2.vllm.test
HTTP/1.1 200 OK
Server: APISIX/3.18.0
model id: smollm2-135m
```

The APISIX Admin API showed one route, one service, and one upstream whose
upstream endpoint was the vLLM pod `10.16.0.246:8000`.

Reusable configuration and the smoke check live under
`docs/deploy/apisix/`.
