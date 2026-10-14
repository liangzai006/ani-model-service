package importer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
)

// HTTPSourceAdapter downloads one explicitly selected repository file. The
// file is supplied as repo_id#file in the ImportModel request; no model or
// filename is hard-coded by the service.
type HTTPSourceAdapter struct {
	BaseURL     string
	Client      *http.Client
	Resolve     func(repoID, revision, file string) string
	ManifestURL func(repoID, revision string) string
	// DefaultRevision is provider-specific because Hugging Face uses main
	// while ModelScope's default branch is master.
	DefaultRevision string
	// ManifestKind selects the wire format used by ListFiles.
	ManifestKind string
}

func (a *HTTPSourceAdapter) ResolveMetadata(_ context.Context, req Request) (Metadata, error) {
	repo, err := splitRepo(req.RepoID)
	if err != nil {
		return Metadata{}, err
	}
	version := a.revision(req.Revision)
	modelID := path.Base(repo)
	if modelID == "." || modelID == "/" || modelID == "" {
		return Metadata{}, fmt.Errorf("%w: model id is empty", ErrProvider)
	}
	return Metadata{ExternalModelID: modelID, Version: version}, nil
}

func NewHuggingFaceAdapter(baseURL string, client *http.Client) *HTTPSourceAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://huggingface.co"
	}
	return &HTTPSourceAdapter{BaseURL: strings.TrimRight(baseURL, "/"), Client: client, DefaultRevision: "main", ManifestKind: "huggingface", Resolve: func(repo, revision, file string) string {
		return strings.TrimRight(baseURL, "/") + "/" + escapePath(repo) + "/resolve/" + url.PathEscape(revision) + "/" + escapePath(file) + "?download=true"
	}, ManifestURL: func(repo, revision string) string {
		return strings.TrimRight(baseURL, "/") + "/api/models/" + escapePath(repo) + "/revision/" + url.PathEscape(revision) + "?blobs=true"
	}}
}

func NewModelScopeAdapter(baseURL string, client *http.Client) *HTTPSourceAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://www.modelscope.cn"
	}
	return &HTTPSourceAdapter{BaseURL: strings.TrimRight(baseURL, "/"), Client: client, DefaultRevision: "master", ManifestKind: "modelscope", Resolve: func(repo, revision, file string) string {
		return strings.TrimRight(baseURL, "/") + "/models/" + escapePath(repo) + "/resolve/" + url.PathEscape(revision) + "/" + escapePath(file)
	}, ManifestURL: func(repo, revision string) string {
		return strings.TrimRight(baseURL, "/") + "/api/v1/models/" + escapePath(repo) + "/repo/files?Revision=" + url.QueryEscape(revision) + "&Recursive=true"
	}}
}

func (a *HTTPSourceAdapter) revision(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	if a.DefaultRevision != "" {
		return a.DefaultRevision
	}
	return "main"
}

func (a *HTTPSourceAdapter) Fetch(ctx context.Context, req Request) (Result, error) {
	content, err := a.FetchContent(ctx, req)
	if err != nil {
		return Result{}, err
	}
	if content.Body != nil {
		content.Body.Close()
	}
	return content.Result, nil
}

func (a *HTTPSourceAdapter) FetchContent(ctx context.Context, req Request) (ContentResult, error) {
	repo, file, err := splitRepoFile(req.RepoID)
	if err != nil {
		return ContentResult{}, err
	}
	revision := a.revision(req.Revision)
	if a.Resolve == nil {
		return ContentResult{}, fmt.Errorf("%w: resolver is not configured", ErrProvider)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, a.Resolve(repo, revision, file), nil)
	if err != nil {
		return ContentResult{}, fmt.Errorf("%w: create download request: %v", ErrProvider, err)
	}
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return ContentResult{}, fmt.Errorf("%w: download: %v", ErrProvider, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		resp.Body.Close()
		return ContentResult{}, fmt.Errorf("%w: download returned status %d", ErrProvider, resp.StatusCode)
	}
	return ContentResult{Result: Result{
		ObjectRef: repo + "/" + revision + "/" + file,
		Format:    formatForFile(file),
		SizeBytes: resp.ContentLength,
	}, Body: resp.Body, ContentType: resp.Header.Get("Content-Type")}, nil
}

func (a *HTTPSourceAdapter) ListFiles(ctx context.Context, req Request) ([]File, error) {
	repo, err := splitRepo(req.RepoID)
	if err != nil {
		return nil, err
	}
	revision := a.revision(req.Revision)
	if a.ManifestURL == nil {
		return nil, fmt.Errorf("%w: manifest resolver is not configured", ErrProvider)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, a.ManifestURL(repo, revision), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: create manifest request: %v", ErrProvider, err)
	}
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: manifest request: %v", ErrProvider, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: manifest returned status %d", ErrProvider, resp.StatusCode)
	}
	var payload struct {
		Siblings []struct {
			Path string `json:"rfilename"`
			Size *int64 `json:"size"`
		} `json:"siblings"`
		Data struct {
			Files []struct {
				Path string `json:"Path"`
				Size int64  `json:"Size"`
				Type string `json:"Type"`
			} `json:"Files"`
		} `json:"Data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: decode manifest: %v", ErrProvider, err)
	}
	files := make([]File, 0, len(payload.Siblings)+len(payload.Data.Files))
	if a.ManifestKind == "modelscope" {
		for _, file := range payload.Data.Files {
			if strings.TrimSpace(file.Path) == "" || strings.EqualFold(file.Type, "tree") {
				continue
			}
			files = append(files, File{Path: file.Path, Size: file.Size})
		}
	} else {
		for _, sibling := range payload.Siblings {
			if strings.TrimSpace(sibling.Path) == "" {
				continue
			}
			size := int64(-1)
			if sibling.Size != nil {
				size = *sibling.Size
			}
			files = append(files, File{Path: sibling.Path, Size: size})
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: manifest contains no files", ErrProvider)
	}
	return files, nil
}

type searchCursor struct {
	Page int `json:"page"`
}

type huggingFaceSearchCursor struct {
	Token string `json:"token"`
}

func encodeSearchCursor(page int) string {
	if page < 1 {
		page = 1
	}
	data, _ := json.Marshal(searchCursor{Page: page})
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeSearchCursor(value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 1, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid search cursor", modelbiz.ErrInvalidSearchRequest)
	}
	var cursor searchCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.Page < 1 {
		return 0, fmt.Errorf("%w: invalid search cursor", modelbiz.ErrInvalidSearchRequest)
	}
	return cursor.Page, nil
}

func encodeHuggingFaceSearchCursor(token string) string {
	data, _ := json.Marshal(huggingFaceSearchCursor{Token: token})
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeHuggingFaceSearchCursor(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("%w: invalid search cursor", modelbiz.ErrInvalidSearchRequest)
	}
	var cursor huggingFaceSearchCursor
	if err := json.Unmarshal(data, &cursor); err != nil || strings.TrimSpace(cursor.Token) == "" {
		return "", fmt.Errorf("%w: invalid search cursor", modelbiz.ErrInvalidSearchRequest)
	}
	return cursor.Token, nil
}

type modelScopeSearchResponse struct {
	Success *bool  `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Models     []modelScopeSearchModel `json:"models"`
		TotalCount *int                    `json:"total_count"`
		PageNumber int                     `json:"page_number"`
		PageSize   int                     `json:"page_size"`
	} `json:"data"`
	Models     []modelScopeSearchModel `json:"models"`
	TotalCount *int                    `json:"total_count"`
	PageNumber int                     `json:"page_number"`
	PageSize   int                     `json:"page_size"`
}

type modelScopeSearchModel struct {
	ID              json.RawMessage `json:"id"`
	RepoID          string          `json:"repo_id"`
	Path            string          `json:"path"`
	Name            string          `json:"name"`
	DisplayName     string          `json:"display_name"`
	DisplayNameAlt  string          `json:"displayName"`
	Description     string          `json:"description"`
	Tags            []string        `json:"tags"`
	Revision        string          `json:"revision"`
	LastModified    string          `json:"last_modified"`
	LastModifiedAlt string          `json:"lastModified"`
	Size            int64           `json:"size"`
	SizeBytes       int64           `json:"size_bytes"`
	FileSize        int64           `json:"file_size"`
	Tasks           []string        `json:"tasks"`
}

type huggingFaceSearchModel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	CardData    struct {
		Description string `json:"description"`
	} `json:"cardData"`
	Tags         []string `json:"tags"`
	PipelineTag  string   `json:"pipeline_tag"`
	Revision     string   `json:"sha"`
	LastModified string   `json:"lastModified"`
	Size         int64    `json:"size"`
	SizeBytes    int64    `json:"size_bytes"`
}

// Search queries the provider's fixed model-list endpoint and returns only
// catalog metadata. It never returns download URLs or provider credentials.
func (a *HTTPSourceAdapter) Search(ctx context.Context, request modelbiz.SearchRequest) (modelbiz.SearchPage, error) {
	if err := request.Validate(); err != nil {
		return modelbiz.SearchPage{}, err
	}
	if request.Source != a.ManifestKind {
		return modelbiz.SearchPage{}, fmt.Errorf("%w: source does not match adapter", ErrProvider)
	}
	page := 1
	cursor := ""
	var err error
	if a.ManifestKind == "huggingface" {
		cursor, err = decodeHuggingFaceSearchCursor(request.Cursor)
	} else {
		page, err = decodeSearchCursor(request.Cursor)
	}
	if err != nil {
		return modelbiz.SearchPage{}, err
	}
	endpoint, err := a.searchURLWithCursor(request, page, cursor)
	if err != nil {
		return modelbiz.SearchPage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return modelbiz.SearchPage{}, fmt.Errorf("%w: create search request: %v", ErrProvider, err)
	}
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return modelbiz.SearchPage{}, fmt.Errorf("%w: search request: %v", ErrProvider, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return modelbiz.SearchPage{}, fmt.Errorf("%w: search returned status %d", ErrProvider, resp.StatusCode)
	}
	var items []modelbiz.SearchResult
	var totalCount *int
	currentPage := page
	pageSize := request.Limit
	switch a.ManifestKind {
	case "modelscope":
		var payload modelScopeSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return modelbiz.SearchPage{}, fmt.Errorf("%w: decode search response: %v", ErrProvider, err)
		}
		if payload.Success != nil && !*payload.Success {
			if strings.TrimSpace(payload.Message) == "" {
				return modelbiz.SearchPage{}, fmt.Errorf("%w: search response reported failure", ErrProvider)
			}
			return modelbiz.SearchPage{}, fmt.Errorf("%w: search response: %s", ErrProvider, payload.Message)
		}
		models := payload.Data.Models
		if models == nil {
			models = payload.Models
		}
		totalCount = payload.Data.TotalCount
		if totalCount == nil {
			totalCount = payload.TotalCount
		}
		if payload.Data.PageNumber > 0 {
			currentPage = payload.Data.PageNumber
		} else if payload.PageNumber > 0 {
			currentPage = payload.PageNumber
		}
		if payload.Data.PageSize > 0 {
			pageSize = payload.Data.PageSize
		} else if payload.PageSize > 0 {
			pageSize = payload.PageSize
		}
		items = make([]modelbiz.SearchResult, 0, len(models))
		for _, item := range models {
			result := a.modelScopeResult(item)
			if strings.TrimSpace(result.RepoID) == "" {
				continue
			}
			items = append(items, result)
		}
	case "huggingface":
		var payload []huggingFaceSearchModel
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return modelbiz.SearchPage{}, fmt.Errorf("%w: decode search response: %v", ErrProvider, err)
		}
		items = make([]modelbiz.SearchResult, 0, len(payload))
		for _, item := range payload {
			items = append(items, a.huggingFaceResult(item))
		}
	default:
		return modelbiz.SearchPage{}, fmt.Errorf("%w: unsupported search provider", ErrProvider)
	}
	result := modelbiz.SearchPage{Items: items}
	if a.ManifestKind == "huggingface" {
		token, err := huggingFaceNextCursor(resp, a.BaseURL)
		if err != nil {
			return modelbiz.SearchPage{}, err
		}
		if token != "" {
			result.HasMore = true
			result.NextCursor = encodeHuggingFaceSearchCursor(token)
		}
	} else if (totalCount != nil && currentPage*pageSize < *totalCount) || (totalCount == nil && len(items) >= request.Limit) {
		result.HasMore = true
		result.NextCursor = encodeSearchCursor(currentPage + 1)
	}
	return result, nil
}

// huggingFaceNextCursor extracts the opaque cursor from Hugging Face's
// same-origin Link header. Offset pagination is not part of the Hub contract.
func huggingFaceNextCursor(resp *http.Response, baseURL string) (string, error) {
	link := strings.TrimSpace(resp.Header.Get("Link"))
	if link == "" {
		return "", nil
	}
	base, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("%w: invalid search base URL", ErrProvider)
	}
	for _, value := range strings.Split(link, ",") {
		value = strings.TrimSpace(value)
		start, end := strings.IndexByte(value, '<'), strings.IndexByte(value, '>')
		if start < 0 || end <= start {
			continue
		}
		relNext := false
		for _, param := range strings.Split(value[end+1:], ";") {
			if strings.EqualFold(strings.TrimSpace(param), `rel="next"`) || strings.EqualFold(strings.TrimSpace(param), "rel=next") {
				relNext = true
			}
		}
		if !relNext {
			continue
		}
		next, err := url.Parse(strings.TrimSpace(value[start+1 : end]))
		if err != nil {
			return "", fmt.Errorf("%w: invalid next link: %v", ErrProvider, err)
		}
		if !next.IsAbs() {
			next = base.ResolveReference(next)
		}
		if next.Scheme != base.Scheme || next.Host != base.Host || next.EscapedPath() != "/api/models" {
			return "", fmt.Errorf("%w: next link is outside provider model endpoint", ErrProvider)
		}
		token := next.Query().Get("cursor")
		if strings.TrimSpace(token) == "" {
			return "", fmt.Errorf("%w: next link has no cursor", ErrProvider)
		}
		return token, nil
	}
	return "", nil
}

func (a *HTTPSourceAdapter) searchURL(request modelbiz.SearchRequest, page int) (string, error) {
	return a.searchURLWithCursor(request, page, "")
}

func (a *HTTPSourceAdapter) searchURLWithCursor(request modelbiz.SearchRequest, page int, cursor string) (string, error) {
	base := strings.TrimRight(a.BaseURL, "/")
	if base == "" {
		return "", fmt.Errorf("%w: search base URL is empty", ErrProvider)
	}
	values := url.Values{}
	values.Set("search", request.Keyword)
	switch a.ManifestKind {
	case "modelscope":
		if request.Owner != "" {
			values.Set("owner", request.Owner)
		}
		if request.Task != "" {
			values.Set("filter.task", request.Task)
		}
		if request.Library != "" {
			values.Set("filter.library", request.Library)
		}
		values.Set("page_size", strconv.Itoa(request.Limit))
		if page > 1 {
			values.Set("page_number", strconv.Itoa(page))
		}
		return base + "/openapi/v1/models?" + values.Encode(), nil
	case "huggingface":
		if request.Owner != "" {
			values.Set("author", request.Owner)
		}
		if request.Task != "" {
			values.Set("pipeline_tag", request.Task)
		}
		if request.Library != "" {
			// Hub list_models exposes tag/library filtering through `filter`; it
			// does not define a `library` query parameter.
			values.Set("filter", request.Library)
		}
		values.Set("limit", strconv.Itoa(request.Limit))
		if cursor != "" {
			values.Set("cursor", cursor)
		}
		return base + "/api/models?" + values.Encode(), nil
	default:
		return "", fmt.Errorf("%w: unsupported search provider", ErrProvider)
	}
}

func (a *HTTPSourceAdapter) modelScopeResult(item modelScopeSearchModel) modelbiz.SearchResult {
	repoID := strings.TrimSpace(item.RepoID)
	if repoID == "" {
		repoID = modelScopeID(item.ID)
	}
	if repoID == "" {
		repoID = strings.TrimSpace(item.Path)
	}
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = path.Base(repoID)
	}
	displayName := strings.TrimSpace(item.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(item.DisplayNameAlt)
	}
	if displayName == "" {
		displayName = name
	}
	// Search metadata does not pin a revision; imports use ModelScope's
	// provider default branch (master).
	revision := a.revision("")
	updated := parseProviderTime(item.LastModified)
	if updated.IsZero() {
		updated = parseProviderTime(item.LastModifiedAlt)
	}
	size := item.FileSize
	if size == 0 {
		size = item.SizeBytes
	}
	if size == 0 {
		size = item.Size
	}
	tags := append([]string(nil), item.Tags...)
	for _, task := range item.Tasks {
		if task != "" {
			tags = appendUnique(tags, task)
		}
	}
	return modelbiz.SearchResult{Source: "modelscope", RepoID: repoID, Name: name, DisplayName: displayName, Description: item.Description, Revision: revision, SizeBytes: size, Tags: tags, UpdatedAt: updated}
}

func modelScopeID(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	// Numeric ids identify a catalog row, not a repository. Keep them out of
	// RepoID unless the API also provides repo_id/path.
	return ""
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (a *HTTPSourceAdapter) huggingFaceResult(item huggingFaceSearchModel) modelbiz.SearchResult {
	repoID := strings.TrimSpace(item.ID)
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = path.Base(repoID)
	}
	displayName := strings.TrimSpace(item.DisplayName)
	if displayName == "" {
		displayName = name
	}
	tags := append([]string(nil), item.Tags...)
	if tag := strings.TrimSpace(item.PipelineTag); tag != "" {
		seen := false
		for _, existing := range tags {
			if existing == tag {
				seen = true
				break
			}
		}
		if !seen {
			tags = append(tags, tag)
		}
	}
	description := item.Description
	if description == "" {
		description = item.CardData.Description
	}
	// Search metadata does not pin a revision; import defaults to HF's main.
	revision := a.revision("")
	size := item.SizeBytes
	if size == 0 {
		size = item.Size
	}
	return modelbiz.SearchResult{Source: "huggingface", RepoID: repoID, Name: name, DisplayName: displayName, Description: description, Revision: revision, SizeBytes: size, Tags: tags, UpdatedAt: parseProviderTime(item.LastModified)}
}

func parseProviderTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func splitRepoFile(repoID string) (string, string, error) {
	parts := strings.SplitN(repoID, "#", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("%w: repo_id must be repo#file", ErrProvider)
	}
	if strings.Contains(parts[0], "..") || strings.Contains(parts[1], "..") {
		return "", "", fmt.Errorf("%w: invalid repository file", ErrProvider)
	}
	return strings.Trim(parts[0], "/"), strings.Trim(parts[1], "/"), nil
}

func splitRepo(repoID string) (string, error) {
	value := strings.TrimSpace(repoID)
	if strings.Contains(value, "#") {
		parts := strings.SplitN(value, "#", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
			return "", fmt.Errorf("%w: invalid repository reference", ErrProvider)
		}
		value = parts[0]
	}
	if value == "" || strings.Contains(value, "..") {
		return "", fmt.Errorf("%w: invalid repository", ErrProvider)
	}
	return strings.Trim(value, "/"), nil
}

func escapePath(value string) string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return path.Join(parts...)
}

func formatForFile(file string) string {
	switch {
	case strings.HasSuffix(file, ".gguf"):
		return "gguf"
	case strings.HasSuffix(file, ".safetensors"):
		return "safetensors"
	default:
		return "pytorch"
	}
}
