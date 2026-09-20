package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
	"github.com/liangzai006/ani-model-service/internal/biz/storage"
	workbiz "github.com/liangzai006/ani-model-service/internal/biz/work"
	"github.com/liangzai006/ani-model-service/internal/data/importer"
	kube "github.com/liangzai006/ani-model-service/internal/data/kubernetes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var ErrImportJobResultMissing = errors.New("import job result is missing")

// KubernetesImportExecutor turns a claimed durable task into one deterministic
// Kubernetes Job. The Job owns provider download and upload; this process only
// observes it and finalizes the immutable artifact after the result is verified.
type KubernetesImportExecutor struct {
	Jobs               *kube.ImportJobClient
	Storage            storage.Port
	Artifacts          modelbiz.ArtifactStore
	Checksums          modelbiz.VersionChecksumStore
	Providers          importer.Registry
	Namespace          string
	Image              string
	ServiceAccount     string
	MinIOSecretName    string
	ProviderSecretName string
	MinIOEndpoint      string
	MinIOBucket        string
	MinIOTenantBuckets bool
	MinIOSecure        bool
	StorageClass       string
	StorageSize        string
	PollInterval       time.Duration
}

func (e KubernetesImportExecutor) Execute(ctx context.Context, task workbiz.Task) error {
	if e.Jobs == nil || e.Storage == nil {
		return errors.New("kubernetes import executor dependencies are not configured")
	}
	if manager, ok := e.Storage.(storage.BucketManager); ok {
		if err := manager.EnsureBucket(ctx, task.TenantID); err != nil {
			return err
		}
	}
	spec, pvc, err := e.jobSpec(ctx, task)
	if err != nil {
		return err
	}
	if _, err := e.ensurePVC(ctx, pvc); err != nil {
		return err
	}
	job, err := e.Jobs.Get(ctx, spec.Namespace, spec.Name)
	if err != nil {
		if !isNotFound(err) {
			return err
		}
		job, err = e.Jobs.Create(ctx, spec)
		if err != nil && !isAlreadyExists(err) {
			return err
		}
		if err != nil {
			job, err = e.Jobs.Get(ctx, spec.Namespace, spec.Name)
			if err != nil {
				return err
			}
		}
	}
	_ = job
	interval := e.PollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	for {
		status, statusErr := e.Jobs.Status(ctx, spec.Namespace, spec.Name)
		if statusErr != nil {
			return statusErr
		}
		switch status.Phase {
		case kube.JobSucceeded:
			return e.finalizeJob(ctx, task, spec.Namespace, spec.Name)
		case kube.JobFailed:
			return fmt.Errorf("import job failed: %s: %s", status.Reason, status.Message)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (e KubernetesImportExecutor) jobSpec(ctx context.Context, task workbiz.Task) (kube.ImportJobSpec, *corev1.PersistentVolumeClaim, error) {
	if task.TenantID == "" || task.ID == "" || task.Source == "" || task.RepoID == "" {
		return kube.ImportJobSpec{}, nil, errors.New("import task identity and source are required")
	}
	if e.Namespace == "" || e.Image == "" {
		return kube.ImportJobSpec{}, nil, errors.New("import Job namespace and image are required")
	}
	name := importJobName(task.ID, task.AttemptCount)
	pvcName := name + "-stage"
	storageSize, err := e.storageSize(ctx, task)
	if err != nil {
		return kube.ImportJobSpec{}, nil, err
	}
	quantity, err := resource.ParseQuantity(storageSize)
	if err != nil {
		return kube.ImportJobSpec{}, nil, fmt.Errorf("import storage size: %w", err)
	}
	objectKey := "imports/" + task.ID + "/model.tar"
	env := []corev1.EnvVar{
		{Name: "ANI_IMPORT_SOURCE", Value: task.Source},
		{Name: "ANI_IMPORT_REPO_ID", Value: task.RepoID},
		{Name: "ANI_IMPORT_REVISION", Value: task.Revision},
		{Name: "ANI_IMPORT_TENANT_ID", Value: task.TenantID},
		{Name: "ANI_IMPORT_TASK_ID", Value: task.ID},
		{Name: "ANI_IMPORT_OBJECT_KEY", Value: objectKey},
		{Name: "ANI_IMPORT_STAGING_DIR", Value: "/staging/model"},
		{Name: "ANI_IMPORT_STORAGE_SIZE", Value: storageSize},
		{Name: "ANI_MINIO_ENDPOINT", Value: e.MinIOEndpoint},
		{Name: "ANI_MINIO_BUCKET", Value: e.MinIOBucket},
		{Name: "ANI_MINIO_TENANT_BUCKETS", Value: fmt.Sprint(e.MinIOTenantBuckets)},
		{Name: "ANI_MINIO_SECURE", Value: fmt.Sprint(e.MinIOSecure)},
	}
	spec := kube.ImportJobSpec{
		Namespace:             e.Namespace,
		Name:                  name,
		Image:                 e.Image,
		Command:               []string{"/ani-model-import"},
		Env:                   env,
		ServiceAccountName:    e.ServiceAccount,
		Labels:                map[string]string{"ani.liangzai006.io/task-id": task.ID, "ani.liangzai006.io/tenant-id": task.TenantID},
		Annotations:           map[string]string{"ani.liangzai006.io/import-object": objectKey},
		BackoffLimit:          int32ptr(0),
		ActiveDeadlineSeconds: int64ptr(7 * 24 * 60 * 60),
		Volumes:               []corev1.Volume{{Name: "staging", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName}}}},
		VolumeMounts:          []corev1.VolumeMount{{Name: "staging", MountPath: "/staging"}},
	}
	for _, secretName := range []string{e.MinIOSecretName, e.ProviderSecretName} {
		if secretName == "" {
			continue
		}
		spec.EnvFrom = append(spec.EnvFrom, corev1.EnvFromSource{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: secretName}}})
	}
	var storageClassName *string
	if strings.TrimSpace(e.StorageClass) != "" {
		storageClassName = &e.StorageClass
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: e.Namespace, Name: pvcName, Labels: spec.Labels, Annotations: map[string]string{"ani.liangzai006.io/import-task": task.ID}},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: storageClassName,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: quantity}},
		},
	}
	return spec, pvc, nil
}

const (
	giBytes                = int64(1 << 30)
	minImportHeadroomBytes = int64(10 << 30)
)

func (e KubernetesImportExecutor) storageSize(ctx context.Context, task workbiz.Task) (string, error) {
	if configured := strings.TrimSpace(e.StorageSize); configured != "" {
		quantity, err := resource.ParseQuantity(configured)
		if err != nil {
			return "", fmt.Errorf("import storage size: %w", err)
		}
		return quantity.String(), nil
	}
	if e.Providers == nil {
		return "", errors.New("ANI_IMPORT_STORAGE_SIZE is required when provider manifest is unavailable")
	}
	provider, err := e.Providers.Resolve(task.Source)
	if err != nil {
		return "", fmt.Errorf("estimate import storage: %w; set ANI_IMPORT_STORAGE_SIZE explicitly", err)
	}
	manifest, ok := provider.(importer.ManifestSource)
	if !ok {
		return "", errors.New("ANI_IMPORT_STORAGE_SIZE is required when provider manifest is unavailable")
	}
	size, ok := estimateManifestSize(ctx, manifest, task)
	if !ok {
		return "", errors.New("cannot determine model size from provider manifest; set ANI_IMPORT_STORAGE_SIZE explicitly")
	}
	return storageQuantity(size), nil
}

func estimateManifestSize(ctx context.Context, source importer.ManifestSource, task workbiz.Task) (int64, bool) {
	files, err := source.ListFiles(ctx, importer.Request{RepoID: task.RepoID, Revision: task.Revision})
	if err != nil || len(files) == 0 {
		return 0, false
	}
	wanted := ""
	if _, file, ok := strings.Cut(task.RepoID, "#"); ok {
		wanted = strings.Trim(file, "/")
	}
	var total int64
	for _, file := range files {
		if wanted != "" && file.Path != wanted {
			continue
		}
		if file.Size < 0 || file.Size > (int64(^uint64(0)>>1)-minImportHeadroomBytes) {
			return 0, false
		}
		if total > int64(^uint64(0)>>1)-file.Size {
			return 0, false
		}
		total += file.Size
	}
	if total <= 0 {
		return 0, false
	}
	headroom := total / 5
	if headroom < minImportHeadroomBytes {
		headroom = minImportHeadroomBytes
	}
	if total > int64(^uint64(0)>>1)-headroom {
		return 0, false
	}
	return total + headroom, true
}

func storageQuantity(bytes int64) string {
	units := (bytes + giBytes - 1) / giBytes
	return fmt.Sprintf("%dGi", units)
}

func (e KubernetesImportExecutor) ensurePVC(ctx context.Context, desired *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	_, err := e.Jobs.GetPVC(ctx, desired.Namespace, desired.Name)
	if err == nil {
		return desired, nil
	}
	if !isNotFound(err) {
		return nil, err
	}
	created, createErr := e.Jobs.CreatePVC(ctx, desired)
	if createErr != nil && !isAlreadyExists(createErr) {
		return nil, createErr
	}
	if createErr != nil {
		return e.Jobs.GetPVC(ctx, desired.Namespace, desired.Name)
	}
	return created, nil
}

func (e KubernetesImportExecutor) finalizeJob(ctx context.Context, task workbiz.Task, namespace, name string) error {
	logs, err := e.Jobs.Logs(ctx, namespace, name)
	if err != nil {
		return err
	}
	result, err := parseImportResult(logs)
	if err != nil {
		return err
	}
	exists, err := e.Storage.ObjectExists(ctx, task.TenantID, result.ObjectRef)
	if err != nil {
		return err
	}
	if !exists {
		return ErrObjectMissing
	}
	if err := e.Storage.VerifyChecksum(ctx, task.TenantID, result.ObjectRef, result.ChecksumSHA256); err != nil {
		return err
	}
	base := ImportExecutor{Artifacts: e.Artifacts, Checksums: e.Checksums}
	if err := base.persistArtifact(ctx, task, result); err != nil {
		return err
	}
	if e.Checksums != nil {
		if err := e.Checksums.SetChecksum(ctx, task.TenantID, task.VersionID, result.ChecksumSHA256, result.SizeBytes); err != nil {
			return err
		}
	}
	return nil
}

func parseImportResult(logs string) (importer.Result, error) {
	const prefix = "ANI_IMPORT_RESULT "
	lines := strings.Split(logs, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		var result struct {
			ObjectRef string `json:"object_ref"`
			Format    string `json:"format"`
			SizeBytes int64  `json:"size_bytes"`
			SHA256    string `json:"sha256"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, prefix)), &result); err != nil {
			return importer.Result{}, fmt.Errorf("decode import job result: %w", err)
		}
		if result.ObjectRef == "" || len(result.SHA256) != 64 || result.SizeBytes < 0 {
			return importer.Result{}, ErrImportJobResultMissing
		}
		return importer.Result{ObjectRef: result.ObjectRef, Format: result.Format, SizeBytes: result.SizeBytes, ChecksumSHA256: result.SHA256}, nil
	}
	return importer.Result{}, ErrImportJobResultMissing
}

func importJobName(taskID string, attempt int) string {
	clean := strings.ToLower(strings.ReplaceAll(taskID, "-", ""))
	if attempt < 0 {
		attempt = 0
	}
	suffix := fmt.Sprintf("-a%d", attempt)
	maxTaskIDLength := 63 - len("ani-import-") - len(suffix)
	if len(clean) > maxTaskIDLength {
		clean = clean[:maxTaskIDLength]
	}
	return "ani-import-" + clean + suffix
}

func int32ptr(v int32) *int32 { return &v }
func int64ptr(v int64) *int64 { return &v }

// These wrappers keep the executor testable with client-go fake errors without
// importing Kubernetes API error types into the worker's domain package.
func isNotFound(err error) bool      { return apierrors.IsNotFound(err) }
func isAlreadyExists(err error) bool { return apierrors.IsAlreadyExists(err) }
