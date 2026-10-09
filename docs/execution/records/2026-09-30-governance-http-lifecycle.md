# Governance HTTP → Model / KServe lifecycle acceptance

This run used only `https://192.168.102.68:6443`, with explicit kubeconfig
`/tmp/ani-newcluster-governance.kubeconfig`. Governance was reached through
`http://192.168.102.68:30095`; inference calls used the peer Higress gateway at
`http://192.168.102.68:30090`. No database records were manually changed, no
old-cluster resources were accessed, and no source commit or push was made.

## Actual coverage

- Unauthenticated Model and Inference list requests returned HTTP 401.
- Temporary tenants 8 and 9 were created through the platform HTTP API.
  Tenant B could not read tenant A's model, version, import task, inference
  service, or operation. Cross-tenant download and retry requests were rejected.
- Model create/get/list, version create/get/list/delete, ModelScope import and
  task lookup, signed upload URL, upload confirmation and download URL passed.
  Repeated create/import requests returned the same model/task; repeated
  Inference create returned the original operation.
- ModelScope `Qwen/Qwen2-0.5B-Instruct` import task
  `2191085c-a1d5-49b5-910a-c8e9428a5b9a` completed on its first attempt.
  Version `e69497a4-9b4f-4421-a4c6-0ad799b82a8b` became ready with
  `999602688` bytes and SHA256
  `e1fc761d399f82372857717c6b38c87ad238f9dc85cf033111b7b2d07bb89e1c`.
- Inference service `8d5587fc-8c4f-4183-a59a-1bfa9f77f9d9`, name and served
  alias `http-qwen-092902`, used KServe `InferenceService` and caller-provided
  vLLM CPU command/args. No GPU request was supplied.
- Higress requests supplied only the JSON model field, without a routing
  header. Real nonempty model generation returned HTTP 200 after creation,
  after start, and after update. Stop withdrew the route and invocation
  returned HTTP 404. Restart reached ready/published; this run did not record
  a separate inference request between restart completion and update.

All six operations finished with `phase=succeeded`, `step=complete`, and empty
error fields. Times below are UTC.

| Operation | Generation | Operation ID | Completed |
| --- | --- | --- | --- |
| Create | 1 | `d65bf7c0-0d6f-4797-b5e2-69c902425053` | 01:36:59 |
| Stop | 2 | `ae423e64-21e6-4077-93f9-aa067cb20441` | 01:39:54 |
| Start | 3 | `c7ae88c7-4dee-449e-9e5f-1e4f807b3522` | 01:45:04 |
| Restart | 4 | `0aaca237-bdf3-4487-8e75-8fb88281a9ce` | 01:52:09 |
| Update | 5 | `efb59078-466e-4a73-a952-51af5d253417` | 01:58:44 |
| Delete | 6 | `283f3616-e5e0-4163-ad0a-180a9d90d465` | 02:01:44 |

Update changed CPU requests and limits from 4 to 3, retaining 8Gi memory.
The final running GET showed generation/appliedGeneration 5 and
`ready/published`. This disproves applying the old legacy Deployment
stale-generation failure to this current KServe resource-update path. Changing
to a different ModelVersion was not tested.

Deletion removes the service from list results, but GET by ID deliberately
returns the tombstone with `desiredState=deleted`, rather than HTTP 404.
Model deletion subsequently succeeded and Model GET returned HTTP 404.

## Observations and limits

- Successful operations sometimes passed through a transient
  `operation state CAS failed` retry. They recovered without intervention;
  this does not establish process-crash or permanent-failure recovery.
- The first upload-confirm test supplied unsupported model format `tar` and
  correctly received HTTP 400. Tar is the transport archive format; the
  accepted model format was `pytorch`. This 10KiB fixture tested storage and
  checksum behavior only, not model validity or inference.
- The first update request included modelVersionId but omitted its FieldMask
  entry and correctly received HTTP 400. The corrected request was accepted.
- Signed object URLs contain the internal MinIO service hostname. The storage
  round-trip required a development-machine port-forward preserving the signed
  Host header. This is not evidence that an external browser can use those URLs
  directly; a client-reachable signing endpoint remains deployment work.
- Successful failed-task retry, concurrent command rejection, controlled
  service-process interruption, permanent failed-create cancellation, and
  switching model versions were not tested. This is not a claim that every
  success and error case of all 23 HTTP routes has passed.
- No new IAM, trusted-identity, TLS, external quota, GPU or unrelated Kubeflow
  integration was added. Source code and deployment images were unchanged in
  this run, so compilation gates were not rerun as a substitute for HTTP tests.

## Cleanup

The delete API removed KServe workloads and publication resources. Four
generations' materialization Jobs/PVCs/download Secrets remained; these were
explicitly removed by the exact service-id label. The import Job/staging PVC
was removed by exact task-id. These were operator cleanup actions, not proof
of automatic garbage collection. Temporary Model and both tenants were deleted
through HTTP; the two uploaded/imported objects were removed by exact MinIO key.

The existing `smollm2-135m-cpu-real` deployment was retained, with UID
`eb367ef4-a6d6-4b8e-873d-a57b00143caa` and its existing vLLM image unchanged.
Raw HTTP evidence, credentials and temporary scripts remain outside Git under
`/tmp/ani-http-accept-20260930`; credentials and signed URLs are not included in
this record.
