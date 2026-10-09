# Governance HTTP full lifecycle regression — 2026-10-03

This record covers only the new cluster from `.clusterenv`:

- Kubernetes API: `https://192.168.102.68:6443`
- Governance business gateway: `http://192.168.102.68:30095`
- Higress inference gateway: `http://192.168.102.68:30090`

The old environment was not contacted or modified. No source commit or push
was made.

## Request path

1. Create two temporary tenants with the public Governance API.
2. Create a model and version, then import `Qwen/Qwen2-0.5B-Instruct` from
   ModelScope through `POST /api/v1/models/{model_id}:import`.
3. Create a KServe-backed inference service with
   `POST /api/v1/inference-services`.
4. Invoke the generated service through Higress
   `POST /v1/completions`, using the JSON `model` alias.
5. Exercise update, stop, start, restart and delete through the Governance
   HTTP API and poll each operation to `succeeded/complete`.

## Results

- Import completed at 999,602,688 bytes with SHA256
  `e1fc761d399f82372857717c6b38c87ad238f9dc85cf033111b7b2d07bb89e1c`.
- Service alias: `http-qwen-1003-111430`.
- The rendered image was the pinned CPU vLLM digest and the complete engine
  command supplied by the request was preserved, including
  `--served-model-name http-qwen-1003-111430`.
- Create, update, stop, start, restart and delete operations all completed.
- Higress returned HTTP 200 with non-empty generated text before and after
  update, after start, and after restart. While stopped it returned HTTP 404.
- The first two update requests were correctly rejected by API validation
  because the temporary helper omitted required update-mask fields. The
  request was corrected to include `modelVersionId`, `resource`, `replicas`,
  `runtime`, `engine`, and `servedModelName`; no manual resource or database
  edits were made.
- Delete removed the Deployment, Service, HTTPRoute and all four temporary
  materialization Job/PVC generations automatically. The model/version and
  both temporary tenants were then deleted through Governance HTTP.
- The Higress rate-limit plugin retained the pre-existing `smollm2-135m` key
  at QPS 10 and remained enabled. A baseline `smollm2-135m` completion still
  returned HTTP 200.

## Boundaries

This verifies the real CPU 0.5B single-replica path. The cluster reports no
GPU, so GPU execution, multi-node distributed inference, large-model capacity,
and accelerator integration were not tested.

Redacted request/response evidence is retained locally under
`/tmp/ani-http-accept-20261003`.
