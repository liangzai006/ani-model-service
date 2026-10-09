# KServe LWS real API validation — 2026-09-24

This validation used only the new cluster context `kubernetes-admin@kubekey`
(`192.168.102.68:6443`). No old-cluster resource was changed and no commit was
created.

The Inference image was rebuilt as a static binary and deployed at:

`docker.changqingyun.cn/ani/inference-service:ani-inference-new-20260924-lws-r3@sha256:1e46f1114e9c3cc403b110ec969563844daa959e7310f2d4a19fd629297e7924`

Migration `000015_inference_kserve_llm_binding.sql` was applied only to the new
cluster's `ani_inference` database. The migration allows durable bindings for
`KServeLLMInferenceService`.

The first real request used the existing ready ModelVersion
`7e4b355c-9cbd-4f5e-ba98-c75e3aadedc1` and supplied `runtime.provider=kserve`,
`RUNTIME_MODE_LEADER_WORKER_SET`, `workerReplicas=1`, a caller-owned image and
command, and 250m CPU/256Mi memory per Pod. Service
`de14fc07-3702-4b75-a3a1-ec7cfb1de153` reached `runtimePhase=ready`.

KServe created an LLMI, a native LWS of size 2, and ready Pods on
`ani-k8s-worker-2` and `ani-k8s-cp`. The generated workload Service exposed
both endpoints and the HTTPRoute was `Accepted` and `ResolvedRefs`.

The observer initially rejected this valid rollout because KServe appended the
`/mnt/models` PVC mount, LWS environment variables and Kubernetes defaults.
It now compares caller-owned fields exactly while allowing those controller
owned additions. Full Inference tests pass.

A second request created `lws-api-gateway` and reached `succeeded`. A real
request through Higress returned HTTP 200:

`POST http://192.168.102.68:30090/v1/completions`

with `x-higress-llm-model: lws-api-gateway` returned the LWS probe payload.
This validates API acceptance, model materialization, KServe LLMI/LWS
reconciliation, generated Service routing and Higress publication.

Both test services were then deleted through the real Delete API. Their LLMI,
LWS, Pods, HTTPRoutes, PVCs, Jobs and Secrets were removed by exact service-id
ownership labels. The existing `smollm2-135m-cpu-real` Deployment-backed model
service remains ready. No GPU is present, so distributed GPU model inference
remains pending.
