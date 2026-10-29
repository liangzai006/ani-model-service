package storage

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/liangzai006/ani-model-service/internal/biz/model"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func factsArchive(t *testing.T, mutate func(*tar.Header)) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := tar.NewWriter(&out)
	for _, name := range []string{"config.json", "model.safetensors", "tokenizer.json", "tokenizer_config.json"} {
		content := []byte(`{"model_type":"qwen2"}`)
		h := &tar.Header{Name: name, Mode: 0600, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if mutate != nil {
			mutate(h)
		}
		if err := writer.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := writer.Write(content); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestMinIOFinetuningFactsPinsOriginalVersionAndRejectsLatest(t *testing.T) {
	for _, mode := range []string{"valid", "no version", "wrong returned version", "changed bytes"} {
		t.Run(mode, func(t *testing.T) {
			raw := factsArchive(t, nil)
			h := sha256.Sum256(raw)
			digest := hex.EncodeToString(h[:])
			gets := 0
			heads := 0
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/models/tenant/model.tar" {
					t.Errorf("unexpected object %s", r.URL)
					w.WriteHeader(404)
					return
				}
				version := "fixed-v1"
				if mode == "no version" {
					version = "null"
				}
				if r.Method == http.MethodGet && mode == "wrong returned version" {
					version = "latest-v2"
				}
				w.Header().Set("x-amz-version-id", version)
				w.Header().Set("ETag", `"etag-is-not-content-sha256"`)
				w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
				w.Header().Set("Content-Length", fmt.Sprint(len(raw)))
				switch r.Method {
				case http.MethodHead:
					heads++
				case http.MethodGet:
					gets++
					if r.URL.Query().Get("versionId") != "fixed-v1" {
						t.Errorf("GET did not pin original version: %s", r.URL)
					}
					body := append([]byte{}, raw...)
					if mode == "changed bytes" {
						body[len(body)-1] = 1
					}
					_, _ = w.Write(body)
				default:
					t.Errorf("facts attempted write %s", r.Method)
					w.WriteHeader(405)
				}
			}))
			defer endpoint.Close()
			client, err := minio.New(strings.TrimPrefix(endpoint.URL, "http://"), &minio.Options{Region: "us-east-1", Creds: credentials.NewStaticV4("access", "secret", ""), Secure: false})
			if err != nil {
				t.Fatal(err)
			}
			adapter := &MinIOAdapter{client: client, bucket: "models", finetuningContentLimit: 1 << 20}
			got, err := adapter.InspectFinetuningArtifact(context.Background(), "tenant", "model.tar", int64(len(raw)), digest)
			if mode == "valid" {
				if err != nil || got.VersionID != "fixed-v1" || got.Bucket != "models" || got.Key != "tenant/model.tar" || gets != 1 || heads != 1 {
					t.Fatalf("got=%+v err=%v GET=%d HEAD=%d", got, err, gets, heads)
				}
			} else if err == nil {
				t.Fatalf("%s accepted", mode)
			}
			if mode == "no version" && gets != 0 {
				t.Fatal("read mutable latest object")
			}
		})
	}
}
func TestFinetuningInventoryHashesActualArchiveAndMembers(t *testing.T) {
	raw := factsArchive(t, nil)
	hash := sha256.Sum256(raw)
	got, err := inspectFinetuningArchive(context.Background(), bytes.NewReader(raw), int64(len(raw)), hex.EncodeToString(hash[:]), 1<<20)
	if err != nil || len(got.Files) != 4 || got.Files[0].Path != "config.json" || got.SizeBytes != int64(len(raw)) || !bytes.Equal(got.ConfigJSON, []byte(`{"model_type":"qwen2"}`)) {
		t.Fatalf("inventory=%+v err=%v", got, err)
	}
	for _, file := range got.Files {
		h := sha256.Sum256([]byte(`{"model_type":"qwen2"}`))
		if file.SHA256 != hex.EncodeToString(h[:]) {
			t.Fatal("invented member hash")
		}
	}
}
func TestFinetuningInventoryRejectsUnsafeOrChangedBytes(t *testing.T) {
	for _, test := range []struct {
		name        string
		mutation    func(*tar.Header)
		appendBytes []byte
		wrongHash   bool
		sizeDelta   int64
	}{
		{name: "parent", mutation: func(h *tar.Header) { h.Name = "../" + h.Name }},
		{name: "link", mutation: func(h *tar.Header) { h.Typeflag = tar.TypeSymlink; h.Linkname = "elsewhere"; h.Size = 0 }},
		{name: "duplicate", mutation: func(h *tar.Header) { h.Name = "config.json" }},
		{name: "trailing data", appendBytes: []byte("hidden")},
		{name: "digest", wrongHash: true}, {name: "length", sizeDelta: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := append(factsArchive(t, test.mutation), test.appendBytes...)
			h := sha256.Sum256(raw)
			digest := hex.EncodeToString(h[:])
			if test.wrongHash {
				digest = string(bytes.Repeat([]byte("a"), 64))
			}
			if _, err := inspectFinetuningArchive(context.Background(), bytes.NewReader(raw), int64(len(raw))+test.sizeDelta, digest, 1<<20); err == nil {
				t.Fatal("unsafe bytes accepted")
			}
		})
	}
}

func TestFinetuningInventoryAcceptsShardsAndNativeTokenizerWithoutInventedFiles(t *testing.T) {
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	members := []struct{ name, body string }{
		{"config.json", `{"model_type":"llama","architectures":["LlamaForCausalLM"],"max_position_embeddings":8192}`},
		{"model.safetensors.index.json", `{"weight_map":{"layer.a":"model-00001-of-00002.safetensors","layer.b":"model-00001-of-00002.safetensors","layer.c":"model-00002-of-00002.safetensors"}}`},
		{"model-00001-of-00002.safetensors", "actual-first-shard"},
		{"model-00002-of-00002.safetensors", "actual-second-shard"},
		{"tokenizer.model", "native-sentencepiece-boundary-bytes"},
		{"tokenizer_config.json", `{"tokenizer_class":"LlamaTokenizer","model_max_length":8192,"chat_template":"{{ messages }}"}`},
		{"chat_template.jinja", "{{ messages }}"},
	}
	for _, m := range members {
		if err := tw.WriteHeader(&tar.Header{Name: m.name, Mode: 0600, Typeflag: tar.TypeReg, Size: int64(len(m.body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(m.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	raw := out.Bytes()
	digest := sha256.Sum256(raw)
	got, err := inspectFinetuningArchive(context.Background(), bytes.NewReader(raw), int64(len(raw)), hex.EncodeToString(digest[:]), 1<<20)
	if err != nil || len(got.Files) != len(members) {
		t.Fatalf("REAL_SHARDED_NATIVE_SOURCE_MISSING: files=%d err=%v", len(got.Files), err)
	}
	for _, f := range got.Files {
		if f.Path == "model.safetensors" || f.Path == "tokenizer.json" {
			t.Fatal("invented single-weight/tokenizer file")
		}
	}
}

func TestFinetuningInventoryContentLimitMeasuresPayloadNotTarContainer(t *testing.T) {
	raw := factsArchive(t, nil)
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	logical := int64(4 * len(`{"model_type":"qwen2"}`))
	got, err := inspectFinetuningArchive(context.Background(), bytes.NewReader(raw), int64(len(raw)), digest, logical)
	if err != nil || len(got.Files) != 4 || int64(len(raw)) <= logical {
		t.Fatalf("B=L should admit independent tar overhead: %v", err)
	}
	_, err = inspectFinetuningArchive(context.Background(), bytes.NewReader(raw), int64(len(raw)), digest, logical-1)
	var violation *model.FinetuningLimitViolation
	if !errors.As(err, &violation) || violation.Reason != "CONTENT_LIMIT_EXCEEDED" || violation.Actual != logical || violation.Limit != logical-1 {
		t.Fatalf("measured content excess not classified: %v", err)
	}
}
func TestFinetuningInventoryCountsHiddenPAXWithinRawArchiveOverhead(t *testing.T) {
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	// archive/tar hides each physical PAX header from its ordinary Next API.
	for i := 0; i < 45; i++ {
		h := &tar.Header{Name: fmt.Sprintf("entry-%d", i), Mode: 0600, Size: 0, Typeflag: tar.TypeReg, Format: tar.FormatPAX, PAXRecords: map[string]string{"comment": strings.Repeat("x", 512<<10)}}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	raw := out.Bytes()
	sum := sha256.Sum256(raw)
	_, err := inspectFinetuningArchive(context.Background(), bytes.NewReader(raw), int64(len(raw)), hex.EncodeToString(sum[:]), 32<<20)
	var violation *model.FinetuningLimitViolation
	if !errors.As(err, &violation) || violation.LimitName != "max_archive_overhead_bytes" {
		t.Fatalf("hidden PAX bytes omitted from C bound: %v", err)
	}
}

func TestFinetuningArchiveEnvelopeLimitIsTypedBeforeSourceAccess(t *testing.T) {
	const limit int64 = 1024
	bound := limit + finetuningTarOverhead
	digest := strings.Repeat("a", 64)
	client, err := minio.New("127.0.0.1:1", &minio.Options{Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &MinIOAdapter{client: client, bucket: "models", finetuningContentLimit: limit}
	for _, check := range []struct {
		name string
		run  func() error
	}{
		{"provider", func() error {
			_, err := adapter.InspectFinetuningArtifact(context.Background(), "tenant", "model.tar", bound+1, digest)
			return err
		}},
		{"archive", func() error {
			_, err := inspectFinetuningArchive(context.Background(), nil, bound+1, digest, limit)
			return err
		}},
	} {
		t.Run(check.name, func(t *testing.T) {
			var violation *model.FinetuningLimitViolation
			if err := check.run(); !errors.As(err, &violation) || violation.Reason != "STRUCTURE_LIMIT_EXCEEDED" || violation.LimitName != "tar_read_bound" || violation.Actual != bound+1 || violation.Limit != bound {
				t.Fatalf("known archive envelope excess lost its bound: %v", err)
			}
		})
	}
}

type interruptedFactsReader struct{ err error }

func (r interruptedFactsReader) Read([]byte) (int, error) { return 0, r.err }

func TestFinetuningArchiveIncompleteObservationIsRetryable(t *testing.T) {
	raw := factsArchive(t, nil)
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	disconnected := errors.New("source connection interrupted")
	for _, test := range []struct {
		name string
		body io.Reader
	}{
		{"header disconnect", io.MultiReader(bytes.NewReader(raw[:100]), interruptedFactsReader{disconnected})},
		{"header short EOF", bytes.NewReader(raw[:100])},
		{"member disconnect", io.MultiReader(bytes.NewReader(raw[:520]), interruptedFactsReader{disconnected})},
		{"padding short EOF", bytes.NewReader(raw[:len(raw)-512])},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := inspectFinetuningArchive(context.Background(), test.body, int64(len(raw)), digest, 1<<20)
			if !errors.Is(err, model.ErrFinetuningFactsUnavailable) || errors.Is(err, model.ErrFinetuningBaseInvalid) {
				t.Fatalf("incomplete source was classified as model content: %v", err)
			}
		})
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := inspectFinetuningArchive(ctx, bytes.NewReader(raw), int64(len(raw)), digest, 1<<20); !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, model.ErrFinetuningBaseInvalid) {
		t.Fatalf("earlier deadline lost its category: %v", err)
	}
}

func TestFinetuningCompleteMalformedArchiveRemainsModelInvalid(t *testing.T) {
	raw := factsArchive(t, nil)
	raw[148] = '!'
	sum := sha256.Sum256(raw)
	_, err := inspectFinetuningArchive(context.Background(), bytes.NewReader(raw), int64(len(raw)), hex.EncodeToString(sum[:]), 1<<20)
	if !errors.Is(err, model.ErrFinetuningBaseInvalid) || errors.Is(err, model.ErrFinetuningFactsUnavailable) {
		t.Fatalf("complete malformed header lost its category: %v", err)
	}
}

func TestFinetuningArchivePathLimitHasStructureDetails(t *testing.T) {
	raw := factsArchive(t, func(header *tar.Header) { header.Name = strings.Repeat("x", 1025) })
	sum := sha256.Sum256(raw)
	_, err := inspectFinetuningArchive(context.Background(), bytes.NewReader(raw), int64(len(raw)), hex.EncodeToString(sum[:]), 1<<20)
	var violation *model.FinetuningLimitViolation
	if !errors.As(err, &violation) || violation.Reason != "STRUCTURE_LIMIT_EXCEEDED" || violation.LimitName != "max_relative_path_bytes" || violation.Actual != 1025 || violation.Limit != 1024 {
		t.Fatalf("path guard lost its actual and limit: %v", err)
	}
}
