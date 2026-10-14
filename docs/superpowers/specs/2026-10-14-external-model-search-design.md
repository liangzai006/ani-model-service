# ModelScope/Hugging Face 外部模型检索设计

## 目标

导入模型时，用户可以在前端输入模型名称或关键词，直接检索 ModelScope、Hugging
Face 的公开模型目录。用户选择候选模型后，再创建本地 Model/ModelVersion 并使用
现有 ImportModel 流程导入整个模型仓库。

搜索结果只用于选择，不直接创建数据库记录，也不启动下载任务。

## 当前缺口

现有 `ListModels.keyword` 只搜索当前租户已经保存的本地模型。ModelScope 和
Hugging Face Provider 只接受精确的 `repo_id`，用于读取文件清单和下载内容，没有
外部目录搜索接口。前端因此必须事先知道完整的仓库标识。

## 方案与边界

搜索由 Model 服务负责，Governance 只提供业务入口和字段转发。Provider 的 HTTP
访问、响应转换和上游错误都留在 Model 服务；前端不直接访问两个外部平台。

新增独立的 `Searcher` 能力，不改变现有下载用的 `SourceAdapter` 契约：

```go
type SearchRequest struct {
	Keyword string
	Owner   string
	Task    string
	Library string
	Limit   int32
	Cursor  string
}

type SearchResult struct {
	Source         string
	RepoID         string
	Name           string
	DisplayName    string
	Description    string
	Revision       string
	SizeBytes      int64 // 0 表示上游未提供精确大小
	Tags           []string
	UpdatedAt      time.Time
}

type SearchPage struct {
	Items      []SearchResult
	NextCursor string
	HasMore    bool
}

type Searcher interface {
	Search(context.Context, SearchRequest) (SearchPage, error)
}
```

Provider 只能由固定的 `source` 枚举选择，不能接受调用方提供的 URL，以避免把搜索
接口变成任意地址代理。ModelScope 使用配置的 ModelScope base URL 和其模型搜索
接口，Hugging Face 使用配置的 Hugging Face base URL 和模型列表接口。搜索请求沿用
现有的 `ANI_MODELSCOPE_BASE_URL`、`ANI_HUGGINGFACE_BASE_URL`，本设计不增加新的
环境变量。

## API 契约

Model gRPC 新增 `SearchExternalModels`，请求包含：

- `source`：`modelscope` 或 `huggingface`；
- `keyword`：必填，限制长度，按上游支持的名称/全文关键字搜索；
- `owner`、`task`、`library`：可选筛选项；
- 现有游标分页字段。

响应返回候选的 provider、`repo_id`、名称、描述、默认 revision、标签、更新时间和
可用的大小信息。没有精确大小时返回 0；选择后的导入任务会通过 manifest 再获取
实际文件大小。响应不返回下载地址、访问密钥或签名 URL。

Governance 增加同等的 catalog 查询和 HTTP 映射，只透传请求和响应字段，不复制
Provider 逻辑。已有的本地 `ListModels` 继续保留，不能把两个查询混为一个接口：
前者是外部候选，后者是租户已入库资源。

## 业务流程

```text
前端输入关键词
    -> Governance SearchExternalModels
    -> Model SearchExternalModels
    -> ModelScope/Hugging Face
    <- 候选列表（不落库）

前端选择候选
    -> CreateModel（保存本地模型和外部 repo_id）
    -> CreateModelVersion（提交引擎类型、镜像、完整启动命令）
    -> ImportModel（source、repo_id、默认 revision、model_id、model_version_id）
    -> Kubernetes Import Job
    -> Storage -> artifact -> ModelVersion ready
```

导入整个仓库。未指定 revision 时，ModelScope 使用 `master`，Hugging Face 使用
`main`。搜索接口不会让用户先选单个文件；现有 manifest/importer 负责完整仓库的
文件清单、归档、上传、校验和 ready 状态。

创建 ModelVersion 时仍必须由调用方提供 `engine_type`、`engine_image` 和完整的
`startup_command`。搜索结果不会推断引擎、镜像或启动命令。

## 错误与安全

- source 不支持、keyword 为空或分页参数无效：返回 `InvalidArgument`；
- 上游超时、限流或不可用：返回可区分的 Provider unavailable 错误，不创建本地资源；
- 只允许固定 Provider 和配置的 base URL，禁止任意 URL、重定向下载和凭据回显；
- 搜索结果不写入 PostgreSQL，只有用户确认并执行导入后才创建本地 Model/Version；
- 搜索和导入使用独立的请求超时，搜索失败不影响已有导入任务。

## 验证

- 为两个 Provider 编写 HTTP mock，验证关键词、筛选、分页、字段转换和默认 revision；
- 验证未知 source、空关键词、过大 limit、上游非 2xx 和 malformed JSON；
- 验证 Model gRPC 与 Governance 转发不改写 `repo_id`、revision 和候选字段；
- 验证搜索不会产生 Model、ModelVersion 或 ImportTask；
- 保持现有 Model import、artifact、ready 和 Inference 链路测试不变。

## 不包含的内容

- 不修改或替换现有导入 Job、Storage 或 Inference 运行时；
- 不自动创建 Model/ModelVersion，不自动开始导入；
- 不为搜索增加新的环境变量或 Provider 凭据体系；
- 不修改旧环境资源，也不在本阶段验证真实外部网络下载。
