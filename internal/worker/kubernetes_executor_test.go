package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/liangzai006/ani-model-service/internal/biz/storage"
	workbiz "github.com/liangzai006/ani-model-service/internal/biz/work"
	"github.com/liangzai006/ani-model-service/internal/data/importer"
	kube "github.com/liangzai006/ani-model-service/internal/data/kubernetes"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
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

func TestKubernetesImportExecutorJobSpecUsesImageEntrypoint(t *testing.T) {
	e := KubernetesImportExecutor{
		Namespace:          "ani-models",
		Image:              "registry.example/ani-model-importer:v1",
		ProviderSecretName: "ani-model-provider",
		StorageClass:       "cephfs",
		Providers:          importer.Registry{"modelscope": manifestSizeSource{}},
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
	if len(spec.Command) != 0 {
		t.Fatalf("command = %#v, want image entrypoint", spec.Command)
	}
	if spec.Namespace != e.Namespace || spec.Image != e.Image || spec.ServiceAccountName != "ani-model-service" {
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
	for _, env := range spec.Env {
		if strings.HasPrefix(env.Name, "ANI_MINIO_") {
			t.Fatalf("unexpected MinIO environment variable: %q", env.Name)
		}
	}
	if len(spec.EnvFrom) != 1 {
		t.Fatalf("envFrom = %#v, want provider secret only", spec.EnvFrom)
	}
}

func TestKubernetesImportExecutorJobSpecUsesUploadURL(t *testing.T) {
	e := KubernetesImportExecutor{
		Namespace: "ani-models",
		Image:     "registry.example/ani-model-importer:v1",
		Providers: importer.Registry{"modelscope": manifestSizeSource{}},
	}
	task := workbiz.Task{
		TenantID: "99999999-9999-4999-8999-999999999999",
		ID:       "11111111-1111-4111-8111-111111111111",
		Source:   "modelscope",
		RepoID:   "Qwen/Qwen3-32B",
		Revision: "master",
	}
	spec, _, err := e.jobSpec(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if got := envValue(spec.Env, "ANI_IMPORT_OBJECT_KEY"); got != "imports/"+task.ID+"/model.tar" {
		t.Fatalf("object key = %q", got)
	}
	var uploadEnv *corev1.EnvVar
	for i := range spec.Env {
		if spec.Env[i].Name == "ANI_IMPORT_UPLOAD_URL" {
			uploadEnv = &spec.Env[i]
			break
		}
	}
	if uploadEnv == nil || uploadEnv.Value != "" || uploadEnv.ValueFrom == nil || uploadEnv.ValueFrom.SecretKeyRef == nil {
		t.Fatalf("upload URL env = %#v, want SecretKeyRef", uploadEnv)
	}
	if uploadEnv.ValueFrom.SecretKeyRef.Key != "ANI_IMPORT_UPLOAD_URL" {
		t.Fatalf("upload URL secret key = %q", uploadEnv.ValueFrom.SecretKeyRef.Key)
	}
	if uploadEnv.ValueFrom.SecretKeyRef.Name == "" {
		t.Fatal("upload URL secret name is empty")
	}
	for _, env := range spec.Env {
		if strings.HasPrefix(env.Name, "ANI_MINIO_") {
			t.Fatalf("unexpected MinIO environment variable: %q", env.Name)
		}
	}
}

func TestKubernetesImportExecutorCreatesUploadSecretForNewJob(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	store := &kubernetesStorageFake{upload: storage.SignedURL{URL: "https://storage.example/upload", ExpiresAt: time.Now().Add(time.Hour)}}
	e := KubernetesImportExecutor{
		Jobs:         kube.NewImportJobClient(clientset),
		Storage:      store,
		Namespace:    "ani-models",
		Image:        "registry.example/ani-model-importer:v1",
		Providers:    importer.Registry{"modelscope": manifestSizeSource{}},
		PollInterval: time.Millisecond,
	}
	task := workbiz.Task{TenantID: "99999999-9999-4999-8999-999999999999", ID: "11111111-1111-4111-8111-111111111111", Source: "modelscope", RepoID: "Qwen/Qwen3-32B", Revision: "master"}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = e.Execute(ctx, task)
	if !store.called {
		t.Fatal("CreateUploadURL was not called")
	}
	if got, want := store.request.ObjectRef, importObjectRef(task); got != want {
		t.Fatalf("upload object ref = %q, want %q", got, want)
	}
	if got, want := store.request.TenantID, task.TenantID; got != want {
		t.Fatalf("upload tenant ID = %q, want %q", got, want)
	}
	if got, want := store.request.ContentType, "application/x-tar"; got != want {
		t.Fatalf("upload content type = %q, want %q", got, want)
	}
	spec, _, err := e.jobSpec(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := clientset.CoreV1().Secrets(e.Namespace).Get(context.Background(), importUploadSecretName(spec.Name), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(secret.Data["ANI_IMPORT_UPLOAD_URL"]); got != store.upload.URL {
		t.Fatalf("upload secret URL = %q, want %q", got, store.upload.URL)
	}
	job, err := clientset.BatchV1().Jobs(e.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "ANI_IMPORT_UPLOAD_URL" {
			if env.Value != "" || env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil || env.ValueFrom.SecretKeyRef.Name != importUploadSecretName(spec.Name) {
				t.Fatalf("Job upload env = %#v, want SecretKeyRef", env)
			}
		}
		if strings.HasPrefix(env.Name, "ANI_MINIO_") {
			t.Fatalf("unexpected MinIO environment variable: %q", env.Name)
		}
	}
}

func TestKubernetesImportExecutorRejectsInvalidUploadURL(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	e := KubernetesImportExecutor{
		Jobs:      kube.NewImportJobClient(clientset),
		Storage:   &kubernetesStorageFake{upload: storage.SignedURL{}},
		Namespace: "ani-models",
		Image:     "registry.example/ani-model-importer:v1",
		Providers: importer.Registry{"modelscope": manifestSizeSource{}},
	}
	task := workbiz.Task{
		TenantID: "99999999-9999-4999-8999-999999999999",
		ID:       "11111111-1111-4111-8111-111111111111",
		Source:   "modelscope",
		RepoID:   "Qwen/Qwen3-32B",
		Revision: "master",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := e.Execute(ctx, task)
	if !errors.Is(err, storage.ErrURLExpired) {
		t.Fatalf("Execute() error = %v, want %v", err, storage.ErrURLExpired)
	}
	jobs, err := clientset.BatchV1().Jobs(e.Namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 0 {
		t.Fatalf("Job count = %d, want no Job for invalid URL", len(jobs.Items))
	}
	secrets, err := clientset.CoreV1().Secrets(e.Namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets.Items) != 0 {
		t.Fatalf("Secret count = %d, want no Secret for invalid URL", len(secrets.Items))
	}
}

func TestKubernetesImportExecutorJobNameIsStablePerAttempt(t *testing.T) {
	e := KubernetesImportExecutor{
		Namespace:    "ani-models",
		Image:        "registry.example/ani-model-importer:v1",
		StorageClass: "cephfs",
		Providers:    importer.Registry{"modelscope": manifestSizeSource{}},
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
	for _, env := range spec.Env {
		if env.Name == "ANI_IMPORT_STORAGE_SIZE" {
			t.Fatalf("unexpected manual storage-size environment: %q", env.Name)
		}
	}
}

func TestKubernetesImportExecutorReusesFailedJobForSameAttempt(t *testing.T) {
	e := KubernetesImportExecutor{
		Namespace:    "ani-models",
		Image:        "registry.example/ani-model-importer:v1",
		StorageClass: "cephfs",
		PollInterval: time.Millisecond,
		Providers:    importer.Registry{"modelscope": manifestSizeSource{}},
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
	store := &kubernetesStorageFake{}
	e.Storage = store
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
	if store.called {
		t.Fatal("existing Job unexpectedly requested a new upload URL")
	}
}

func TestKubernetesImportExecutorRefusesExistingUploadSecret(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	store := &kubernetesStorageFake{upload: storage.SignedURL{URL: "https://storage.example/new", ExpiresAt: time.Now().Add(time.Hour)}}
	e := KubernetesImportExecutor{
		Jobs:      kube.NewImportJobClient(clientset),
		Storage:   store,
		Namespace: "ani-models",
		Image:     "registry.example/ani-model-importer:v1",
		Providers: importer.Registry{"modelscope": manifestSizeSource{}},
	}
	task := workbiz.Task{TenantID: "99999999-9999-4999-8999-999999999999", ID: "11111111-1111-4111-8111-111111111111", Source: "modelscope", RepoID: "Qwen/Qwen3-32B"}
	spec, _, err := e.jobSpec(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	existing := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: e.Namespace, Name: importUploadSecretName(spec.Name)}, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{"ANI_IMPORT_UPLOAD_URL": []byte("https://storage.example/old")}}
	if _, err := clientset.CoreV1().Secrets(e.Namespace).Create(context.Background(), existing, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	err = e.Execute(context.Background(), task)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Execute() error = %v, want existing Secret refusal", err)
	}
	if store.called {
		t.Fatal("existing upload Secret unexpectedly requested a new upload URL")
	}
	jobs, err := clientset.BatchV1().Jobs(e.Namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 0 {
		t.Fatalf("Job count = %d, want no Job when upload Secret already exists", len(jobs.Items))
	}
	got, err := clientset.CoreV1().Secrets(e.Namespace).Get(context.Background(), existing.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data["ANI_IMPORT_UPLOAD_URL"]) != "https://storage.example/old" {
		t.Fatal("existing upload Secret was overwritten")
	}
}

func TestKubernetesImportExecutorCleansUploadSecretAfterFailedJob(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	store := &kubernetesStorageFake{upload: storage.SignedURL{URL: "https://storage.example/upload", ExpiresAt: time.Now().Add(time.Hour)}}
	e := KubernetesImportExecutor{Jobs: kube.NewImportJobClient(clientset), Storage: store, Namespace: "ani-models", Image: "registry.example/ani-model-importer:v1", Providers: importer.Registry{"modelscope": manifestSizeSource{}}, PollInterval: time.Millisecond}
	task := workbiz.Task{TenantID: "99999999-9999-4999-8999-999999999999", ID: "11111111-1111-4111-8111-111111111111", Source: "modelscope", RepoID: "Qwen/Qwen3-32B"}
	spec, pvc, err := e.jobSpec(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.CoreV1().PersistentVolumeClaims(e.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.BatchV1().Jobs(e.Namespace).Create(context.Background(), func() *batchv1.Job {
		job, buildErr := kube.BuildImportJob(spec)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "failed"}}
		return job
	}(), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.CoreV1().Secrets(e.Namespace).Create(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: e.Namespace, Name: importUploadSecretName(spec.Name)}, Data: map[string][]byte{"ANI_IMPORT_UPLOAD_URL": []byte(store.upload.URL)}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	err = e.Execute(context.Background(), task)
	if err == nil || !strings.Contains(err.Error(), "import job failed") {
		t.Fatalf("Execute() error = %v, want failed Job", err)
	}
	if _, err := clientset.CoreV1().Secrets(e.Namespace).Get(context.Background(), importUploadSecretName(spec.Name), metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("upload Secret after failed Job error = %v, want not found", err)
	}
}

func TestKubernetesImportExecutorCleansUploadSecretAfterSucceededJob(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	store := &kubernetesStorageFake{}
	e := KubernetesImportExecutor{Jobs: kube.NewImportJobClient(clientset), Storage: store, Namespace: "ani-models", Image: "registry.example/ani-model-importer:v1", Providers: importer.Registry{"modelscope": manifestSizeSource{}}, PollInterval: time.Millisecond}
	task := workbiz.Task{TenantID: "99999999-9999-4999-8999-999999999999", ID: "11111111-1111-4111-8111-111111111111", Source: "modelscope", RepoID: "Qwen/Qwen3-32B"}
	spec, pvc, err := e.jobSpec(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.CoreV1().PersistentVolumeClaims(e.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	job, err := kube.BuildImportJob(spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.BatchV1().Jobs(e.Namespace).Create(context.Background(), job, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	job.Status.Succeeded = 1
	if _, err := clientset.BatchV1().Jobs(e.Namespace).UpdateStatus(context.Background(), job, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.CoreV1().Secrets(e.Namespace).Create(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: e.Namespace, Name: importUploadSecretName(spec.Name)}, Data: map[string][]byte{"ANI_IMPORT_UPLOAD_URL": []byte("https://storage.example/upload")}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := e.Execute(context.Background(), task); err == nil {
		t.Fatal("Execute() unexpectedly succeeded without import result logs")
	}
	if _, err := clientset.CoreV1().Secrets(e.Namespace).Get(context.Background(), importUploadSecretName(spec.Name), metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("upload Secret after succeeded Job error = %v, want not found", err)
	}
}

func TestKubernetesImportExecutorCleansUploadSecretAfterJobCreateFailure(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("job create failed")
	})
	store := &kubernetesStorageFake{upload: storage.SignedURL{URL: "https://storage.example/upload", ExpiresAt: time.Now().Add(time.Hour)}}
	e := KubernetesImportExecutor{Jobs: kube.NewImportJobClient(clientset), Storage: store, Namespace: "ani-models", Image: "registry.example/ani-modeler:v1", Providers: importer.Registry{"modelscope": manifestSizeSource{}}}
	task := workbiz.Task{TenantID: "99999999-9999-4999-8999-999999999999", ID: "11111111-1111-4111-8111-111111111111", Source: "modelscope", RepoID: "Qwen/Qwen3-32B"}
	spec, _, err := e.jobSpec(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	err = e.Execute(context.Background(), task)
	if err == nil || !strings.Contains(err.Error(), "job create failed") {
		t.Fatalf("Execute() error = %v, want create failure", err)
	}
	if _, err := clientset.CoreV1().Secrets(e.Namespace).Get(context.Background(), importUploadSecretName(spec.Name), metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("upload Secret after Job create failure error = %v, want not found", err)
	}
}

func TestKubernetesImportExecutorCleansSecretWhenRacingJobCreationLoses(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewAlreadyExists(batchv1.Resource("jobs"), action.(k8stesting.CreateAction).GetObject().(*batchv1.Job).Name)
	})
	store := &kubernetesStorageFake{upload: storage.SignedURL{URL: "https://storage.example/upload", ExpiresAt: time.Now().Add(time.Hour)}}
	e := KubernetesImportExecutor{Jobs: kube.NewImportJobClient(clientset), Storage: store, Namespace: "ani-models", Image: "registry.example/ani-model-importer:v1", Providers: importer.Registry{"modelscope": manifestSizeSource{}}}
	task := workbiz.Task{TenantID: "99999999-9999-4999-8999-999999999999", ID: "11111111-1111-4111-8111-111111111111", Source: "modelscope", RepoID: "Qwen/Qwen3-32B"}
	spec, _, err := e.jobSpec(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	err = e.Execute(context.Background(), task)
	if err == nil || !strings.Contains(err.Error(), "get import job") {
		t.Fatalf("Execute() error = %v, want follow-up Job get failure", err)
	}
	if _, err := clientset.CoreV1().Secrets(e.Namespace).Get(context.Background(), importUploadSecretName(spec.Name), metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("upload Secret after Job race error = %v, want not found", err)
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

type kubernetesStorageFake struct {
	upload  storage.SignedURL
	called  bool
	request storage.UploadRequest
}

func (s *kubernetesStorageFake) CreateUploadURL(_ context.Context, request storage.UploadRequest) (storage.SignedURL, error) {
	s.called = true
	s.request = request
	return s.upload, nil
}
func (*kubernetesStorageFake) ObjectExists(context.Context, string, string) (bool, error) {
	return true, nil
}
func (*kubernetesStorageFake) VerifyChecksum(context.Context, string, string, string) error {
	return nil
}
func (*kubernetesStorageFake) CreateDownloadURL(context.Context, storage.DownloadRequest) (storage.SignedURL, error) {
	return storage.SignedURL{}, nil
}
