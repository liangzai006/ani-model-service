# 外部模型搜索实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 ModelScope 和 Hugging Face 增加按模型名称搜索的统一接口，让前端选择外部模型后继续使用现有 ModelVersion 与 ImportModel 流程。

**Architecture:** 在 Model 业务层定义外部搜索端口，Provider HTTP 适配器分别实现 ModelScope 和 Hugging Face 的搜索协议。Model gRPC 暴露搜索方法，Governance 只做 catalog/HTTP 转发；搜索结果不落库，前端确认后创建 Model、ModelVersion，再提交现有 ImportModel。

**Tech Stack:** Go、Kratos gRPC/HTTP、protobuf、Buf、现有 ModelScope/Hugging Face HTTP Provider、Governance catalog API。

## Global Constraints

- 只允许 `modelscope` 和 `huggingface` 两个固定 Provider，不能接受调用方任意 URL。
- ModelScope 默认 revision 为 `master`，Hugging Face 默认 revision 为 `main`。
- 搜索导入整个仓库；搜索不启动下载、不创建 Model/ModelVersion/ImportTask。
- ModelVersion 的 `engine_type`、`engine_image`、`startup_command` 仍由调用方提交，搜索结果不得推断或拼接启动命令。
- 不增加环境变量，复用 `ANI_MODELSCOPE_BASE_URL` 和 `ANI_HUGGINGFACE_BASE_URL`。
- Governance 只转发字段，不复制 Provider 逻辑；Governance 仓库不推送。
- Gov 的所有新增 API 必须先修改 `.proto`，再使用 Governance 固定生成链
  `GOWORK=off make api`（内部执行本地 `tools/bin/gow api`）生成 Go、gRPC、HTTP
  和错误代码；禁止手写或直接编辑 `api/gen/go`。
- 不修改旧环境资源、Storage、Import Job 或 Inference 运行时。

---

### Task 1: 定义外部搜索业务端口

**Files:**
- Create: `/root/kubercon/ani-model-service/internal/biz/model/search.go`
- Test: `/root/kubercon/ani-model-service/internal/biz/model/search_test.go`

**Interfaces:**
- Consumes: Provider 名称、关键词、筛选条件和游标。
- Produces: 被 service 与 importer 共同使用的 `Searcher`、`SearchRequest`、`SearchResult`、`SearchPage` 类型。

- [ ] **Step 1: Write the failing test**

```go
func TestSearchRequestValidatesProviderAndKeyword(t *testing.T) {
	if err := (SearchRequest{Source: "modelscope", Keyword: "qwen", Limit: 20}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (SearchRequest{Source: "unknown", Keyword: "qwen", Limit: 20}).Validate(); err == nil {
		t.Fatal("unknown source accepted")
	}
	if err := (SearchRequest{Source: "modelscope", Keyword: "  ", Limit: 20}).Validate(); err == nil {
		t.Fatal("blank keyword accepted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOWORK=off go test ./internal/biz/model -run TestSearchRequestValidatesProviderAndKeyword -count=1`

Expected: FAIL because the search port and validation do not exist.

- [ ] **Step 3: Write the minimal implementation**

Define `SearchRequest` with `Source`, `Keyword`, `Owner`, `Task`, `Library`, `Limit`, and `Cursor`; define `SearchResult` with `Source`, `RepoID`, `Name`, `DisplayName`, `Description`, `Revision`, `SizeBytes`, `Tags`, and `UpdatedAt`; define `SearchPage` with `Items`, `NextCursor`, and `HasMore`. `Validate` must accept only the two fixed sources, require a trimmed keyword, reject a limit outside 1..50, and reject a cursor longer than 2048 bytes. Define:

```go
type Searcher interface {
	Search(context.Context, SearchRequest) (SearchPage, error)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `GOWORK=off go test ./internal/biz/model -run TestSearchRequestValidatesProviderAndKeyword -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/biz/model/search.go internal/biz/model/search_test.go
git commit -m "feat: define external model search port"
```

### Task 2: Implement ModelScope and Hugging Face search adapters

**Files:**
- Modify: `/root/kubercon/ani-model-service/internal/data/importer/source.go`
- Modify: `/root/kubercon/ani-model-service/internal/data/importer/http_source.go`
- Modify: `/root/kubercon/ani-model-service/internal/data/importer/source_test.go`

**Interfaces:**
- Consumes: `model.SearchRequest` and fixed provider base URLs.
- Produces: `Registry.Search(ctx, request)` implementing the business `model.Searcher` port.

- [ ] **Step 1: Write failing HTTP mock tests**

Add one test for each provider. The ModelScope test must assert a request like
`/openapi/v1/models?search=Qwen&page_size=2` and decode `data.models`; the Hugging Face test
must assert `/api/models?search=Qwen&limit=2` and decode its model list. Both tests must
assert `repo_id`, display name, tags, revision, and `SizeBytes == 0` when the upstream does
not provide a precise total size. Add tests for non-2xx and malformed JSON responses.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOWORK=off go test ./internal/data/importer -run 'Test.*Search' -count=1`

Expected: FAIL because the Registry search method and adapter implementation do not exist.

- [ ] **Step 3: Implement provider search**

Add a `Searcher` lookup beside the existing `SourceAdapter` lookup. Build URLs only from
the configured provider base URL and escaped query parameters. Map ModelScope fields
(`id`, `name`/display metadata, `tags`, `last_modified`) and Hugging Face fields (`id`,
`pipeline_tag`, `tags`, `lastModified`) into `model.SearchResult`. Use the provider default
revision when the response does not include one. Encode the provider page in the opaque
cursor and never expose provider download URLs or credentials.

- [ ] **Step 4: Run focused tests**

Run: `GOWORK=off go test ./internal/data/importer -run 'Test.*Search' -count=1`

Expected: PASS, including non-2xx and malformed JSON cases.

- [ ] **Step 5: Commit**

```bash
git add internal/data/importer/source.go internal/data/importer/http_source.go internal/data/importer/source_test.go
git commit -m "feat: search external model providers"
```

### Task 3: Expose Model gRPC search and wire runtime dependencies

**Files:**
- Modify: `/root/kubercon/ani-model-service/api/model/v1/model.proto`
- Modify: `/root/kubercon/ani-model-service/internal/service/model.go`
- Modify: `/root/kubercon/ani-model-service/cmd/ani-model-service/main.go`
- Modify: `/root/kubercon/ani-model-service/internal/service/model_test.go`
- Generated: `/root/kubercon/ani-model-service/api/model/v1/model.pb.go`
- Generated: `/root/kubercon/ani-model-service/api/model/v1/model_grpc.pb.go`

**Interfaces:**
- Consumes: `model.Searcher` and provider registry from Task 2.
- Produces: `ModelService.SearchExternalModels` with validated tenant-scoped request and transient candidates.

- [ ] **Step 1: Extend the protobuf contract**

Add `rpc SearchExternalModels(SearchExternalModelsRequest) returns (SearchExternalModelsResponse);`
with request fields `source`, `keyword`, `owner`, `task`, `library`, and the existing cursor
page shape. Add response messages for candidates and page metadata. Keep response fields
limited to provider, repo ID, names, description, revision, size, tags, and updated time.

- [ ] **Step 2: Regenerate the pinned output**

Run: `GOWORK=off make api`

Expected: Gov's pinned Buf/protoc chain succeeds and only the expected catalog/admin
generated files change.

- [ ] **Step 3: Write the failing service test**

Add a fake `Searcher` to `ModelService` and assert that `SearchExternalModels` validates
tenant identity, forwards all filters, maps every candidate unchanged, and never calls a
catalog or work store. Add cases for blank keyword, unknown source, invalid limit, provider
failure, and malformed provider result.

- [ ] **Step 4: Run the test to verify it fails**

Run: `GOWORK=off go test ./internal/service -run 'TestSearchExternalModels' -count=1`

Expected: FAIL because the RPC, dependency setter, and handler do not exist.

- [ ] **Step 5: Implement the handler and wiring**

Add a `searchers model.Searcher` field and `SetExternalModelSearcher`/registry wiring to
`ModelService`. The handler must call `identity.RequireTenant`, construct the validated
business request, call the selected Provider searcher, and return an opaque cursor. Map
validation failures to 400 and upstream errors to a provider-unavailable 502. In
`cmd/ani-model-service/main.go`, register the same ModelScope and Hugging Face adapters used
by the importer and attach their search capability to the service.

- [ ] **Step 6: Run focused service tests**

Run: `GOWORK=off go test ./internal/service ./internal/data/importer -run 'Test(SearchExternalModels|.*Search)' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add api/model/v1/model.proto api/model/v1/model.pb.go api/model/v1/model_grpc.pb.go internal/service/model.go internal/service/model_test.go cmd/ani-model-service/main.go
git commit -m "feat: expose external model search"
```

### Task 4: Add Governance catalog pass-through locally

**Files:**
- Modify: `/root/kubercon/ani-governance/api/protos/catalog/service/v1/model.proto`
- Modify: `/root/kubercon/ani-governance/app/admin/service/internal/service/model_service.go`
- Modify: `/root/kubercon/ani-governance/app/admin/service/internal/data/model_client.go`
- Test: `/root/kubercon/ani-governance/app/admin/service/internal/service/model_service_test.go`
- Generated: the catalog protobuf and HTTP output files produced by the pinned Governance generation workflow.

**Interfaces:**
- Consumes: Model gRPC `SearchExternalModels` from Task 3.
- Produces: Governance catalog/HTTP search that copies request and response fields without Provider logic.

- [ ] **Step 1: Extend the catalog protobuf**

Add catalog request/response messages and the corresponding Model service method using the
same field names and cursor semantics as Model. Keep `source` restricted to the two fixed
providers and do not add provider URL or credential fields.

- [ ] **Step 2: Add the ModelClient forwarding method**

Clone the request, inject the trusted tenant metadata exactly as existing ModelClient calls
do, call the Model gRPC method, and return the response unchanged. Do not add an HTTP client
for ModelScope or Hugging Face in Governance.

- [ ] **Step 3: Add the Governance service mapping tests**

Assert that source, keyword, owner/task/library filters, cursor, and every result field are
preserved; assert upstream errors map to the existing Model error mapping and that missing
trusted tenant context is rejected before the downstream call.

- [ ] **Step 4: Regenerate and run Governance focused checks**

Run the repository's pinned generation command for catalog APIs, then run the model service
unit/contract tests and `git diff --check`. Keep all work in a separate local Governance
worktree or otherwise isolate it from the user's existing dirty Governance changes. Do not
push or merge this repository.

- [ ] **Step 5: Record the local-only boundary**

Keep the Governance changes unpushed and report their commit or patch location separately;
the Model repository remains the only repository committed by default.

### Task 4a: Complete the Model management surface

The Model service contract already provides these operations, but the Gov BFF does not
expose them. Add each operation to the catalog proto, `ModelService` proto, generated
registration, `ModelOwnerClient`, mTLS client adapter and service mapping:

- `SearchExternalModels`
- `DeleteModel`
- `DeleteModelVersion`
- `GetUploadURL`
- `GetModelDownloadURL`

Every request must derive tenant and actor from the authenticated Gov principal. Public
requests must not accept a caller-supplied tenant ID or downstream endpoint. Model deletion
must preserve the downstream reference check and error mapping.

Regenerate with `GOWORK=off make api`, then run Gov's focused service/client tests and
verify the generated HTTP registration contains each expected path.

### Task 4b: Complete the Inference management surface

The downstream Inference contract also has lifecycle methods that are not represented in
Gov. Add BFF proto messages, generated routes, trusted mTLS client calls, authorization
permission entries and service mappings for:

- `UpdateInferenceService`
- `StartInferenceService`
- `StopInferenceService`
- `RestartInferenceService`
- `ListOperations`

Keep create/delete quota ownership and asynchronous operation semantics unchanged. Each
operation must carry the authenticated tenant and actor metadata to Inference and must not
allow public callers to forge those fields.

Regenerate with `GOWORK=off make api`; do not hand-edit generated Go output. Add focused
authorization, mapping and HTTP registration tests before any image rebuild.

### Task 5: End-to-end contract tests and documentation

**Files:**
- Modify: `/root/kubercon/ani-model-service/docs/model-inference-fields.md`
- Modify: `/root/kubercon/ani-model-service/README.md`
- Create: `/root/kubercon/ani-model-service/docs/execution/records/2026-10-14-external-model-search.md`
- Test: Model and Governance service/provider test files from Tasks 2–4.

**Interfaces:**
- Consumes: completed search and pass-through APIs.
- Produces: operator-facing flow documentation and verification evidence.

- [ ] **Step 1: Document the frontend sequence**

Show the exact sequence: SearchExternalModels → CreateModel → CreateModelVersion with
engine defaults → ImportModel with selected `source/repo_id/revision` and local IDs. State
that search does not persist or download and that the full repository is imported.

- [ ] **Step 2: Add negative and pagination coverage**

Run provider mock tests, Model service tests, and Governance mapping tests for invalid input,
provider errors, cursor continuation, and empty results. Assert that no local store or import
worker is called by search.

- [ ] **Step 3: Run repository verification**

Model:

```bash
GOWORK=off make verify
```

Governance:

```bash
GOWORK=off go test ./app/admin/service/...
git diff --check
```

If external network or database variables are absent, record the skipped integration tests
instead of claiming live provider or database validation.

- [ ] **Step 4: Commit Model documentation and verification record**

```bash
git add README.md docs/model-inference-fields.md docs/execution/records/2026-10-14-external-model-search.md
git commit -m "docs: record external model search flow"
```
