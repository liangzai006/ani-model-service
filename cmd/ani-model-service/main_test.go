package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	storagedata "github.com/liangzai006/ani-model-service/internal/data/storage"
)

type minioReadinessFake struct {
	bucketChecks int
	tenantID     string
}

func (f *minioReadinessFake) CheckBucket(context.Context) error {
	f.bucketChecks++
	return nil
}

func (f *minioReadinessFake) CheckTenantConnection(_ context.Context, tenantID string) error {
	f.tenantID = tenantID
	return nil
}

func TestCheckMinIOStorageUsesConfiguredReadinessMode(t *testing.T) {
	shared := &minioReadinessFake{}
	if err := checkMinIOStorage(context.Background(), shared, false); err != nil {
		t.Fatal(err)
	}
	if shared.bucketChecks != 1 || shared.tenantID != "" {
		t.Fatalf("shared storage checks = %+v", shared)
	}
	tenant := &minioReadinessFake{}
	if err := checkMinIOStorage(context.Background(), tenant, true); err != nil {
		t.Fatal(err)
	}
	if tenant.bucketChecks != 0 || tenant.tenantID == "" {
		t.Fatalf("tenant storage checks = %+v", tenant)
	}
}

func TestConfigureKubernetesImportUsesStorageGRPCWithoutMinIO(t *testing.T) {
	t.Setenv("ANI_MINIO_ENDPOINT", "")
	t.Setenv("ANI_IMPORT_JOB_IMAGE", "registry.example/ani-model-import:v1")
	t.Setenv("ANI_IMPORT_KUBERNETES_NAMESPACE", "ani-model")
	t.Setenv("ANI_IMPORT_PROVIDER_SECRET", "ani-model-import-provider")
	t.Setenv("ANI_IMPORT_STORAGE_CLASS", "cephfs")
	t.Setenv("ANI_IMPORT_UPLOAD_TTL_SECONDS", "")

	selectedStorage := &storagedata.GRPCAdapter{}
	executor, err := configuredKubernetesImportExecutor(selectedStorage)
	if err != nil {
		t.Fatalf("configure Kubernetes import with Storage gRPC: %v", err)
	}
	if executor.Storage != selectedStorage {
		t.Fatal("Kubernetes import executor did not use the selected Storage gRPC adapter")
	}
	if executor.ProviderSecretName != "ani-model-import-provider" {
		t.Fatalf("ProviderSecretName = %q", executor.ProviderSecretName)
	}
	if executor.StorageClass != "cephfs" {
		t.Fatalf("StorageClass = %q", executor.StorageClass)
	}
	if executor.UploadTTL != 0 {
		t.Fatalf("default Storage gRPC import TTL = %v, want adapter default", executor.UploadTTL)
	}
	t.Setenv("ANI_IMPORT_UPLOAD_TTL_SECONDS", "3600")
	if _, err := configuredKubernetesImportExecutor(selectedStorage); err == nil || !strings.Contains(err.Error(), "Storage gRPC") {
		t.Fatalf("explicit unsupported Storage gRPC import TTL must fail configuration: %v", err)
	}
}

func TestConfigureKubernetesImportUploadTTL(t *testing.T) {
	t.Setenv("ANI_IMPORT_JOB_IMAGE", "registry.example/ani-model-import:v1")
	for _, test := range []struct {
		value   string
		want    time.Duration
		wantErr bool
	}{
		{value: ""},
		{value: "0"},
		{value: "3600", want: time.Hour},
		{value: "604800", want: 7 * 24 * time.Hour},
		{value: "-1", wantErr: true},
		{value: "604801", wantErr: true},
		{value: "invalid", wantErr: true},
	} {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv("ANI_IMPORT_UPLOAD_TTL_SECONDS", test.value)
			executor, err := configuredKubernetesImportExecutor(&storagedata.MinIOAdapter{})
			if (err != nil) != test.wantErr {
				t.Fatalf("configure import upload TTL %q error = %v, wantErr %v", test.value, err, test.wantErr)
			}
			if err == nil && executor.UploadTTL != test.want {
				t.Fatalf("import upload TTL = %v, want %v", executor.UploadTTL, test.want)
			}
		})
	}
}

func TestConfigureKubernetesImportStorageCA(t *testing.T) {
	t.Setenv("ANI_IMPORT_JOB_IMAGE", "registry.example/ani-model-import:v1")
	t.Setenv("ANI_IMPORT_UPLOAD_TTL_SECONDS", "")
	for _, configMap := range []string{"", "ani-model-v21-storage-ca"} {
		t.Run(configMap, func(t *testing.T) {
			t.Setenv("ANI_IMPORT_STORAGE_CA_CONFIGMAP", configMap)
			executor, err := configuredKubernetesImportExecutor(&storagedata.MinIOAdapter{})
			if err != nil {
				t.Fatal(err)
			}
			if executor.StorageCAConfigMap != configMap {
				t.Fatalf("import CA ConfigMap = %q, want %q", executor.StorageCAConfigMap, configMap)
			}
		})
	}
}

func TestRuntimeLoggerUsesKratosRedaction(t *testing.T) {
	var output bytes.Buffer
	logger := newRuntimeLogger(&output)
	logger.Info("redaction check", "token", "secret-token", "args", "secret-payload")

	line := output.String()
	if strings.Contains(line, "secret-token") || strings.Contains(line, "secret-payload") {
		t.Fatalf("runtime logger leaked filtered values: %s", line)
	}
	if strings.Count(line, `"***"`) != 2 {
		t.Fatalf("runtime logger did not use Kratos key filtering: %s", line)
	}
}

func TestRuntimeLoggerIncludesProcessIdentityAndSource(t *testing.T) {
	originalID, originalName, originalVersion := id, Name, Version
	id, Name, Version = "layout-test-instance", "ani-model-service", "layout-test-version"
	t.Cleanup(func() { id, Name, Version = originalID, originalName, originalVersion })

	var output bytes.Buffer
	newRuntimeLogger(&output).Info("identity check")
	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatalf("decode structured log: %v; output=%q", err, output.String())
	}
	for key, want := range map[string]string{
		"msg":             "identity check",
		"service.id":      "layout-test-instance",
		"service.name":    "ani-model-service",
		"service.version": "layout-test-version",
	} {
		if got, _ := record[key].(string); got != want {
			t.Fatalf("log field %s = %q, want %q; record=%v", key, got, want, record)
		}
	}
	if timestamp, _ := record["time"].(string); timestamp == "" {
		t.Fatalf("structured log has no timestamp: %v", record)
	}
	source, ok := record["source"].(map[string]any)
	if !ok || source["file"] == "" || source["line"] == nil {
		t.Fatalf("structured log has no caller source: %v", record)
	}
}

func TestMainProcessRejectsIncompleteConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("external process gate is disabled by -short")
	}
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve command package path")
	}
	commandDir := filepath.Dir(filename)
	repositoryRoot := filepath.Clean(filepath.Join(commandDir, "..", ".."))
	binaryPath := filepath.Join(t.TempDir(), "service")
	build := exec.Command("go", "build", "-trimpath", "-o", binaryPath, ".")
	build.Dir = commandDir
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build process binary: %v\n%s", err, output)
	}

	var stdout, stderr bytes.Buffer
	process := exec.Command(binaryPath, "-conf", filepath.Join(repositoryRoot, "configs"))
	process.Dir = repositoryRoot
	process.Stdout = &stdout
	process.Stderr = &stderr
	process.Env = runtimeEnvironment(
		"ANI_DATABASE_DSN=",
		"ANI_MINIO_ENDPOINT=",
		"ANI_STORAGE_GRPC_ADDR=",
	)
	if err := process.Start(); err != nil {
		t.Fatalf("start process binary: %v", err)
	}
	processDone := make(chan error, 1)
	go func() { processDone <- process.Wait() }()
	select {
	case err := <-processDone:
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 1 {
			t.Fatalf("process exit = %v, want exit code 1; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
		}
		if !strings.Contains(stdout.String(), "configuration validation failed") {
			t.Fatalf("configuration failure was not logged: stdout=%s stderr=%s", stdout.String(), stderr.String())
		}
	case <-time.After(4 * time.Second):
		_ = process.Process.Kill()
		t.Fatal("process did not reject incomplete configuration")
	}
}

func runtimeEnvironment(overrides ...string) []string {
	blocked := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		key, _, _ := strings.Cut(override, "=")
		blocked[key] = struct{}{}
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, found := blocked[key]; !found {
			environment = append(environment, entry)
		}
	}
	return append(environment, overrides...)
}
