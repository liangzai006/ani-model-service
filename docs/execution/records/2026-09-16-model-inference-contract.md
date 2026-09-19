# Model ↔ Inference contract integration

Scope: the Model v1 catalog client, Inference create-time snapshot persistence,
and two independent Kratos gRPC test processes using PostgreSQL and MinIO.
IAM/Gateway are explicitly deferred because those services do not yet exist.

## Corrections

The initial unconnected Inference adapter used `OperationContext.ID` as
`model_version_id` and returned `ModelObservation.Ready` for catalog readiness.
The prepared replacement accepts an explicit version UUID and returns a
snapshot: version/model identity, artifact reference, SHA256, engine and argv.
It never establishes model materialization or runtime readiness.

The Inference composition-root change optionally enables the catalog client
through `ANI_MODEL_GRPC_ADDR`, owns/terminates the client connection, and wraps
the existing PostgreSQL create use case. Model-supplied artifact metadata and
engine defaults become the Inference spec. Conflicting overrides fail closed.
TLS is the default; plaintext requires explicit local-test configuration.

## Verification and current limits

- Focused client tests passed in the isolated Inference working copy, including
  explicit version ID, incomplete/non-ready responses, checksum validation,
  gRPC status propagation, invalid download addresses, and create-time mapping.
- `GOFLAGS=-buildvcs=false GOCACHE=/tmp/ani-go-cache go test ./internal/... ./api/... -count=1`
  passed in that copy (live dependency tests skipped without opt-in).
  Full `go build ./...`, vet of the client/composition root, and the new
  composition-root configuration test also passed. `-buildvcs=false` is needed
  only because this temporary tree has no Git metadata.
- Model's `go test ./tests/contract ./internal/client ./internal/service -count=1`
  and `git diff --check` passed. The live Model-server test skips without opt-in.
- `/tmp/ani-inference-model-client-verified.patch` contains the staged fixes;
  `git -C /root/kubercon/ani-inference-service apply --check` passed. It is based
  on the current sibling working tree and includes its necessary client fix.
- `tests/contract/model_server_test.go` compiles and is opt-in. It provides a real
  Kratos Model handler with PostgreSQL and MinIO plus a test-only Principal.
- `scripts/test-model-inference-contract` starts that Model process, then runs
  Inference's separate integration-test process with its own generated client.
- Host execution is pending. After the user's renewed authorization to execute
  ordinary non-destructive commands automatically, write-back review timed out;
  the permitted retry failed with `Selected model is at capacity`. Both the
  live integration command and a smaller host-only default test command were
  also rejected because the automatic reviewer was at capacity. None of these
  commands executed. No successful two-process/live-dependency result is
  claimed yet; this is a platform approval failure, not a test result.
- The staged Inference tree is `/tmp/ani-inference-contract.hvvqXe`. The sibling
  repository still has the initial client and updated tests until write-back is
  allowed; those tests intentionally fail against the old client interface.
- The copied Model proto differs only in `go_package`. Re-running pinned Buf
  generation in the isolated tree is currently blocked by sandbox dependency
  lookup; generated file hashes remained unchanged. The existing generated
  client compiles in the isolated module, but this is not a regeneration pass.

### Additional checks after renewed authorization

- Full `go vet ./...` passed in both the actual Model repository and the
  isolated Inference tree. Model's `go build -trimpath ./...` and Inference's
  `go build ./...` exited 0. Model build emitted a module stat-cache write
  warning for the read-only shared cache; Inference uses `-buildvcs=false`
  because the isolated tree has no Git metadata.
- Both full `go test ./... -count=1` runs were attempted in the sandbox. All
  Inference internal/API packages, including the new Model client/create
  adapter, passed. Its process-lifecycle test failed when reserving a loopback
  socket (`operation not permitted`). Model failed for the same socket
  restriction in composition/lifecycle, importer HTTP, and runtime tests;
  the other reported packages passed. These runs are **not** full-suite passes.
- The live-dependency tests remain opt-in and skipped in the default runs.
  `make verify` has not been rerun successfully for this pending integration.
- `git diff --check`, shell syntax validation of the contract script, and
  `git apply --check /tmp/ani-inference-model-client-verified.patch` passed.
  The patch changes eight source/documentation files and contains no deletion,
  commit, push, migration, or deployment operation.
- An independent review agent could not start because its selected model was
  at capacity; no independent-review pass is claimed.

Create replay currently performs the Model lookup before the existing local
idempotency transaction, so replay still requires Model availability. The
planned live test checks replay while Model is available; outage-independent
replay is not covered by this change.

The contract test requires a dedicated tenant with an existing ready Model
version and a real artifact smaller than 16 MiB. Supply `E2E_DSN`,
`E2E_INFERENCE_DSN`, `E2E_TENANT_ID`, `E2E_VERSION_ID`, `E2E_MINIO_ENDPOINT`,
`E2E_MINIO_ACCESS`, and `E2E_MINIO_SECRET` through the environment; use
`E2E_MINIO_INSECURE=1` only for the existing plaintext development MinIO.

```sh
MODEL_CONTRACT_E2E=1 bash scripts/test-model-inference-contract /root/kubercon/ani-inference-service
```

The test preserves newly created Inference rows. It verifies neither real IAM,
GPU loading, runtime materialization, quota/publication, nor Kubernetes readiness.

## Final live deployment evidence

- ModelVersion: `d8ebd3cc-efd7-414d-803d-345df4af34a0`; artifact SHA256
  `38f8fe399fa9017f1a8278c4627f328f33c5743cd3e5dda8518cbae8a876bc62`; bytes
  `272455680`; version `12fd25f77366-live1`.
- Inference service: `0f0265c3-9d18-410f-9a1b-851101754799`; operation
  `34909728-bfff-41d4-bded-da38d7a49995`; namespace
  `ani-model-inference-e2e-retry-20260916`.
- Deployment `smollm2-live-20260916-final` is `1/1` ready. Model Job
  `ani-model-3d79e1f0e6ef9a8a82d99df1-fetch` is `Complete 1/1`; PVC is Bound on
  `cephfs`. Published NodePort is `smollm2-live-20260916-final-published:30325`.
- Durable readback: operation `succeeded/complete`, runtime `ready`, publication
  `published`, invocation `healthy`.
- Real completion at `http://10.10.1.67:30325/v1/completions` for prompt
  `The capital of France is` returned ` Paris. Paris is the largest city in`.
