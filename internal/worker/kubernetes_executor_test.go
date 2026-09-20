package worker

import (
	"context"
	"strings"
	"testing"
	"time"

	workbiz "github.com/liangzai006/ani-model-service/internal/biz/work"
	"github.com/liangzai006/ani-model-service/internal/data/importer"
	kube "github.com/liangzai006/ani-model-service/internal/data/kubernetes"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestParseImportResultUsesLastStableResultLine(t *testing.T) {
	result, err := parseImportResult("download complete\nANI_IMPORT_RESULT {\"object_ref\":\"imports/task/model.tar\",\"format\":\"safetensors\",\"size_bytes\":123,\"sha256\":\"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\"}\n")
	if err != nil {
		t.Fatal(err)
	}
	if result.ObjectRef != "imports/task/model.tar" || result.SizeBytes != 123 || result.ChecksumSHA256 == "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestKubernetesImportExecutorJobSpecUsesImportEntrypoint(t *testing.T) {
	e := KubernetesImportExecutor{
		Namespace:          "ani-models",
		Image:              "registry.example/ani-model-importer:v1",
		ServiceAccount:     "ani-model-importer",
		MinIOSecretName:    "ani-minio",
		ProviderSecretName: "ani-model-provider",
		MinIOEndpoint:      "minio.storage:9000",
		MinIOBucket:        "ani-models",
		StorageClass:       "cephfs",
		StorageSize:        "512Gi",
	}
	spec, pvc, err := e.jobSpec(context.Background(), workbiz.Task{
		TenantID: "99999999-9999-4999-8999-999999999999",
		ID:       "11111111-1111-4111-8111-111111111111",
		Source:   "modelscope",
		RepoID:   "Qwen/Qwen3-32B",
		Revision: "master",
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Command == nil || len(spec.Command) != 1 || spec.Command[0] != "/ani-model-import" {
		t.Fatalf("command = %#v", spec.Command)
	}
	if spec.Namespace != e.Namespace || spec.Image != e.Image || spec.ServiceAccountName != e.ServiceAccount {
		t.Fatalf("job identity = %#v", spec)
	}
	if pvc.Name == "" || pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "cephfs" {
		t.Fatalf("pvc = %#v", pvc)
	}
	if got := envValue(spec.Env, "ANI_IMPORT_REPO_ID"); got != "Qwen/Qwen3-32B" {
		t.Fatalf("repo env = %q", got)
	}
	if got := envValue(spec.Env, "ANI_IMPORT_OBJECT_KEY"); got == "" {
		t.Fatal("object key env is empty")
	}
	if got := envValue(spec.Env, "ANI_MINIO_TENANT_BUCKETS"); got != "false" {
		t.Fatalf("tenant bucket mode = %q", got)
	}
	if len(spec.EnvFrom) != 2 {
		t.Fatalf("envFrom = %#v, want MinIO and provider secrets", spec.EnvFrom)
	}
}

func TestKubernetesImportExecutorJobNameIsStablePerAttempt(t *testing.T) {
	e := KubernetesImportExecutor{
		Namespace:    "ani-models",
		Image:        "registry.example/ani-model-importer:v1",
		StorageClass: "cephfs",
		StorageSize:  "512Gi",
	}
	task := workbiz.Task{
		TenantID: "99999999-9999-4999-8999-999999999999",
		ID:       "11111111-1111-4111-8111-111111111111",
		Source:   "modelscope",
		RepoID:   "Qwen/Qwen3-32B",
		Revision: "master",
	}

	task.AttemptCount = 1
	first, _, err := e.jobSpec(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	resumed, _, err := e.jobSpec(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != resumed.Name {
		t.Fatalf("same attempt names differ: %q != %q", first.Name, resumed.Name)
	}

	task.AttemptCount = 2
	retried, _, err := e.jobSpec(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Name == first.Name {
		t.Fatalf("different attempts reused job name %q", retried.Name)
	}
	if got, want := retried.Name, importJobName(task.ID, task.AttemptCount); got != want {
		t.Fatalf("retry job name = %q, want %q", got, want)
	}
}

type manifestSizeSource struct{}

func (manifestSizeSource) Fetch(context.Context, importer.Request) (importer.Result, error) {
	return importer.Result{}, nil
}

func (manifestSizeSource) ListFiles(context.Context, importer.Request) ([]importer.File, error) {
	return []importer.File{{Path: "config.json", Size: 100}, {Path: "weights.safetensors", Size: 900}}, nil
}

func TestKubernetesImportExecutorUsesClusterDefaultAndEstimatesPVCSize(t *testing.T) {
	e := KubernetesImportExecutor{
		Namespace: "ani-models",
		Image:     "registry.example/ani-model-importer:v1",
		Providers: importer.Registry{"modelscope": manifestSizeSource{}},
	}
	spec, pvc, err := e.jobSpec(context.Background(), workbiz.Task{
		TenantID: "99999999-9999-4999-8999-999999999999",
		ID:       "11111111-1111-4111-8111-111111111111",
		Source:   "modelscope",
		RepoID:   "Qwen/Qwen3-32B",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pvc.Spec.StorageClassName != nil {
		t.Fatalf("StorageClassName = %q, want nil for cluster default", *pvc.Spec.StorageClassName)
	}
	if got := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; got.String() != "11Gi" {
		t.Fatalf("estimated storage = %s, want 11Gi", got.String())
	}
	if got := envValue(spec.Env, "ANI_IMPORT_STORAGE_SIZE"); got != "11Gi" {
		t.Fatalf("storage env = %q, want 11Gi", got)
	}
}

func TestKubernetesImportExecutorReusesFailedJobForSameAttempt(t *testing.T) {
	e := KubernetesImportExecutor{
		Namespace:    "ani-models",
		Image:        "registry.example/ani-model-importer:v1",
		StorageClass: "cephfs",
		StorageSize:  "512Gi",
		PollInterval: time.Millisecond,
	}
	task := workbiz.Task{
		TenantID:     "99999999-9999-4999-8999-999999999999",
		ID:           "11111111-1111-4111-8111-111111111111",
		Source:       "modelscope",
		RepoID:       "Qwen/Qwen3-32B",
		Revision:     "master",
		AttemptCount: 1,
	}
	spec, _, err := e.jobSpec(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	clientset := k8sfake.NewSimpleClientset()
	jobs := kube.NewImportJobClient(clientset)
	if _, err := jobs.Create(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	job, err := clientset.BatchV1().Jobs(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	job.Status.Failed = 1
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded"}}
	if _, err := clientset.BatchV1().Jobs(spec.Namespace).UpdateStatus(context.Background(), job, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	e.Jobs = jobs
	e.Storage = &objectFake{}
	err = e.Execute(context.Background(), task)
	if err == nil || !strings.Contains(err.Error(), "BackoffLimitExceeded") {
		t.Fatalf("Execute() error = %v, want failed Job status", err)
	}
	list, err := clientset.BatchV1().Jobs(spec.Namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("Job count = %d, want existing Job reused", len(list.Items))
	}
}

func envValue(envs []corev1.EnvVar, name string) string {
	for _, env := range envs {
		if env.Name == name {
			return env.Value
		}
	}
	return ""
}
