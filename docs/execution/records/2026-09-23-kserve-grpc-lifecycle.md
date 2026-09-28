# KServe business API lifecycle — 2026-09-23

## Scope

This verification used only the new cluster through the explicit context
`kubernetes-admin@kubekey` (`https://192.168.102.68:6443`). The old cluster and
all existing deployment-backed services were left untouched. No IAM, TLS,
trusted identity, quota, Notebook, Trainer, or Pipeline behavior was added.

The change remains uncommitted. Baseline commits are `ani-inference-service`
`f223725` and `ani-model-service` `9cfc05b`.

## Flow verified

1. Model version `7e4b355c-9cbd-4f5e-ba98-c75e3aadedc1` was already `ready`.
   Its artifact is `272455680` bytes. The Model service now returns the
   positive artifact size when an older version row has `size_bytes=0`.
2. A real `CreateInferenceService` gRPC request created service
   `db6bb85d-c55a-4248-a4b3-9425a739a510` named
   `smollm2-kserve-grpc-real` with `runtime.provider=kserve`. The complete
   caller-supplied vLLM command and served alias
   `smollm2-135m-kserve-grpc` were persisted unchanged.
3. Operation `ef9d1d62-8862-494c-8711-223b3b3ab57e` reached `succeeded`.
   The materializer created the CephFS claim and verified the download Job;
   KServe created a RawDeployment predictor and reached Ready.
4. HTTPRoute `ani-pub-3b5055493efa57ed77d0` was Accepted and ResolvedRefs.
   A POST to `http://192.168.102.68:30090/v1/completions` with the fixed path,
   the model header `x-higress-llm-model: smollm2-135m-kserve-grpc`, and the
   same model name returned HTTP 200 and non-empty text. The first cold CPU
   request took about 34 seconds; the response model was the requested alias.
5. A real `DeleteInferenceService` request for the same service reached
   `succeeded` as operation `daf5b5b5-eaaa-4ba6-9412-29bdab4248c5`.
   KServe InferenceService, predictor Deployment/Service, ANI CR, and
   HTTPRoute disappeared. The test-owned materialization PVC, Job, and Secret
   were then removed by their exact service-id ownership label; no unrelated
   resource was selected.

## Images and migrations

- Inference control plane:
  `docker.changqingyun.cn/ani/inference-service:ani-inference-new-20260923-kserve-r4@sha256:dd29cf5a3f1252a0bd93547e7af35f6c9f4b1a4619581c7d498fe608cf87aa3a`
- Model service:
  `docker.changqingyun.cn/ani/model-service:ani-model-new-20260923-r12@sha256:b6ce6ee4db0cf40e736f6f050578f82b1781c40df2af51131afadb1368e4daf2`
- Inference migrations `000013` and `000014` were applied only to the new
  cluster database.

## Verification

Inference `go test ./... -count=1`, `go vet ./...`, static build, and
`git diff --check` pass. Model `make verify` passes. The existing deployment
service `smollm2-135m-cpu-real` and its HTTPRoute remained present and Ready
after the KServe service was deleted.
