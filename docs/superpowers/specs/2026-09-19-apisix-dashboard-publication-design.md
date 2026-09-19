# APISIX Dashboard and Production Publication Design

## Goal

Expose the APISIX built-in Dashboard through a dedicated Admin NodePort, then
make Inference Publication create and withdraw the Kubernetes Gateway API
`HTTPRoute` that APISIX Ingress Controller translates into a data-plane route.

The existing Envoy Gateway remains installed until the APISIX path is accepted.
The first production publication protocol remains HTTP-only because the model
runtime currently exposes an OpenAI-compatible HTTP Service.

## Dashboard

APISIX 3.18 already serves its embedded Dashboard at `/ui/`; the current
Admin Service is `ClusterIP`, so the UI is not reachable outside the cluster.
Create a separate `ingress-apisix/ani-apisix-admin-public` NodePort Service on
an unused fixed port (`30092`) selecting only the APISIX data-plane pods. The
existing `ani-apisix-admin` Service remains ClusterIP and the Admin API keeps
Admin Key authentication. The Dashboard URL is:

```text
http://<node-internal-ip>:30092/ui/
```

Because this Service also exposes the Admin API, deployment documentation must
show the Admin Key requirement and explicitly identify the endpoint as an
administrative interface.

## Publication architecture

`internal/biz/publication` remains the domain port. Extend its durable
`Publication` value with the exact Gateway API identity and backend information
needed by a publisher:

- route namespace/name and Gateway namespace/name;
- external hostname and HTTP path prefix;
- backend Service namespace/name and port;
- URL returned after publication confirmation.

The Kubernetes adapter in `internal/data/kubernetes` implements the port with a
controller-runtime client. `Publish` creates or updates one namespaced
`HTTPRoute` using deterministic labels and an owner-independent identity based
on tenant, service, generation, and operation fencing. `Withdraw` deletes that
same object. `ConfirmPublished` checks `Accepted=True` and `ResolvedRefs=True`;
`ConfirmWithdrawn` returns true only when the object is absent. Conflicts,
missing references, and non-ready conditions remain retryable errors.

The adapter never calls APISIX Admin API directly. APISIX Ingress Controller
watches the HTTPRoute and owns translation to APISIX. This keeps the
publication authority at Kubernetes and makes withdrawal observable through the
same resource conditions.

## Composition and persistence

`cmd/ani-inference-service/main.go` constructs the Kubernetes publication
adapter when `ANI_KUBERNETES_ENABLED=true`, alongside the existing Runtime
Executor. Configuration supplies the APISIX Gateway namespace/name, route
hostname suffix, and path prefix. PostgreSQL persists the resulting URL and
observed phase through the existing `SavePublication` path; no second route
store is introduced.

`CurrentOperation` loads the target-generation publication and passes the
backend data from the durable runtime projection to the adapter. Existing
operation fencing (`LeaseToken`, `FenceOperationID`, `Generation`) remains the
idempotency boundary. Stop, restart, update replacement, and delete continue
to require confirmed withdrawal before runtime removal.

## Verification

- Dashboard Service exposes `/ui/` on NodePort `30092` and Admin API calls
  still require `X-API-KEY`.
- Unit tests prove deterministic HTTPRoute rendering, idempotent publish,
  withdrawal, condition checks, and namespace/name validation.
- Kubernetes envtest proves the adapter observes Accepted/ResolvedRefs and
  absence semantics.
- The live smoke test creates a route to a real vLLM Service, verifies
  `GET /v1/models` through APISIX, then withdraws the route and verifies the
  request no longer reaches the backend.
