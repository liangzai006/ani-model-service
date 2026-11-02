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
	if got := envValue(spec.Env, "HOME"); got != "/staging" {
		t.Fatalf("HOME = %q, want writable staging volume", got)
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
	if got := envValue(spec.Env, "ANI_IMPORT_UPLOAD_URL_FILE"); got != "/var/run/ani-import-upload/ANI_IMPORT_UPLOAD_URL" {
		t.Fatalf("upload URL file = %q", got)
	}
	found := false
	for _, volume := range spec.Volumes {
		if volume.Name == "upload-url" && volume.Secret != nil && volume.Secret.SecretName == importUploadSecretName(spec.Name) {
			found = true
		}
	}
	if !found {
		t.Fatal("missing projected upload Secret")
	}
	for _, env := range spec.Env {
		if strings.HasPrefix(env.Name, "ANI_MINIO_") {
			t.Fatalf("unexpected MinIO environment variable: %q", env.Name)
		}
	}
}

func TestKubernetesImportExecutorJobStorageCA(t *testing.T) {
	for _, configMap := range []string{"", "ani-model-v21-storage-ca"} {
		t.Run(configMap, func(t *testing.T) {
			e := KubernetesImportExecutor{
				Namespace:          "ani-system",
				Image:              "registry.example/ani-model-importer:v1",
				Providers:          importer.Registry{"modelscope": manifestSizeSource{}},
				StorageCAConfigMap: configMap,
			}
			spec, _, err := e.jobSpec(context.Background(), workbiz.Task{
				TenantID: "99999999-9999-4999-8999-999999999999",
				ID:       "11111111-1111-4111-8111-111111111111",
				Source:   "modelscope", RepoID: "HuggingFaceTB/SmolLM2-135M", Revision: "master",
			})
			if err != nil {
				t.Fatal(err)
			}
			job, err := kube.BuildImportJob(spec)
			if err != nil {
				t.Fatal(err)
			}
			pod := job.Spec.Template.Spec
			container := pod.Containers[0]
			var caVolumes []corev1.Volume
			var caMounts []corev1.VolumeMount
			for _, volume := range pod.Volumes {
				if volume.Name == "model-storage-ca" {
					caVolumes = append(caVolumes, volume)
				}
			}
			for _, mount := range container.VolumeMounts {
				if mount.Name == "model-storage-ca" {
					caMounts = append(caMounts, mount)
				}
			}
			if configMap == "" {
				if len(caVolumes) != 0 || len(caMounts) != 0 {
					t.Fatalf("unconfigured import gained a CA mount: volumes=%#v mounts=%#v", pod.Volumes, container.VolumeMounts)
				}
				for _, env := range container.Env {
					if env.Name == "SSL_CERT_FILE" {
						t.Fatal("unconfigured import overrides its image CA bundle")
					}
				}
				return
			}
			if got := envValue(container.Env, "SSL_CERT_FILE"); got != "/var/run/ani-model-storage-ca/ca.crt" {
				t.Fatalf("import SSL_CERT_FILE = %q", got)
			}
			if len(caVolumes) != 1 || len(caMounts) != 1 {
				t.Fatalf("configured import CA mount missing: volumes=%#v mounts=%#v", pod.Volumes, container.VolumeMounts)
			}
			volume, mount := caVolumes[0], caMounts[0]
			if volume.ConfigMap == nil || volume.ConfigMap.Name != configMap || len(volume.ConfigMap.Items) != 1 || volume.ConfigMap.Items[0].Key != "ca.crt" || volume.ConfigMap.Items[0].Path != "ca.crt" {
				t.Fatalf("import CA ConfigMap/key = %#v", volume)
			}
			if mount.Name != volume.Name || mount.MountPath != "/var/run/ani-model-storage-ca" || !mount.ReadOnly {
				t.Fatalf("import CA mount = %#v", mount)
			}
		})
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
		UploadTTL:    time.Hour,
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
	if got, want := store.request.TTL, time.Hour; got != want {
		t.Fatalf("import upload TTL = %v, want %v", got, want)
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
			t.Fatal("upload URL must not be fixed in environment")
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
	if len(list.Items) != 0 {
		t.Fatalf("Job count = %d, want terminal Job cleaned up", len(list.Items))
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
	if _, err := clientset.BatchV1().Jobs(e.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("Job after failed terminal cleanup error = %v, want not found", err)
	}
	if _, err := clientset.CoreV1().PersistentVolumeClaims(e.Namespace).Get(context.Background(), pvc.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("PVC after failed terminal cleanup error = %v, want not found", err)
	}
	var deleted []string
	for _, action := range clientset.Actions() {
		if deleteAction, ok := action.(k8stesting.DeleteAction); ok {
			deleted = append(deleted, deleteAction.GetResource().Resource)
		}
	}
	if got, want := strings.Join(deleted, ","), "jobs,persistentvolumeclaims,secrets"; got != want {
		t.Fatalf("terminal cleanup delete order = %q, want %q", got, want)
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
	if _, err := clientset.BatchV1().Jobs(e.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("Job after succeeded terminal cleanup error = %v, want not found", err)
	}
	if _, err := clientset.CoreV1().PersistentVolumeClaims(e.Namespace).Get(context.Background(), pvc.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("PVC after succeeded terminal cleanup error = %v, want not found", err)
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

func TestRefreshUploadSecretUsesFreshURLAndChecksIdentity(t *testing.T) {
	ctx := context.Background()
	task := workbiz.Task{ID: "11111111-1111-4111-8111-111111111111", TenantID: "99999999-9999-4999-8999-999999999999"}
	name := importUploadSecretName("import-job")
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ani-models", Labels: map[string]string{"ani.liangzai006.io/task-id": task.ID, "ani.liangzai006.io/tenant-id": task.TenantID}, Annotations: map[string]string{"ani.liangzai006.io/import-object": importObjectRef(task), "ani.liangzai006.io/upload-expires-at": time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)}}, Data: map[string][]byte{"ANI_IMPORT_UPLOAD_URL": []byte("https://example/old")}}
	client := k8sfake.NewSimpleClientset(secret)
	store := &kubernetesStorageFake{upload: storage.SignedURL{URL: "https://example/new", ExpiresAt: time.Now().Add(15 * time.Minute)}}
	e := KubernetesImportExecutor{Jobs: kube.NewImportJobClient(client), Storage: store}
	if err := e.refreshUploadSecret(ctx, task, "ani-models", "import-job"); err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().Secrets("ani-models").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data["ANI_IMPORT_UPLOAD_URL"]) != store.upload.URL || !store.called {
		t.Fatal("expired URL was not refreshed")
	}
	got.Labels["ani.liangzai006.io/tenant-id"] = "other"
	got.Annotations["ani.liangzai006.io/upload-expires-at"] = time.Now().Add(time.Minute).Format(time.RFC3339Nano)
	if _, err = client.CoreV1().Secrets("ani-models").Update(ctx, got, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err = e.refreshUploadSecret(ctx, task, "ani-models", "import-job"); err == nil {
		t.Fatal("foreign Secret was accepted")
	}
}

func TestImportJobUsesConfiguredCABundle(t *testing.T) {
	e := KubernetesImportExecutor{Namespace: "ani-models", Image: "import", CAConfigMapName: "public-ca", CAConfigMapKey: "bundle.pem", Providers: importer.Registry{"modelscope": manifestSizeSource{}}}
	task := workbiz.Task{ID: "11111111-1111-4111-8111-111111111111", TenantID: "99999999-9999-4999-8999-999999999999", Source: "modelscope", RepoID: "org/model"}
	spec, _, err := e.jobSpec(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if envValue(spec.Env, "SSL_CERT_FILE") != "/var/run/ani-import-ca/ca-certificates.crt" {
		t.Fatal("missing CA trust path")
	}
	for _, v := range spec.Volumes {
		if v.Name == "import-ca" && v.ConfigMap != nil && v.ConfigMap.Name == "public-ca" && len(v.ConfigMap.Items) == 1 && v.ConfigMap.Items[0].Key == "bundle.pem" {
			return
		}
	}
	t.Fatal("missing configured CA bundle")
}
