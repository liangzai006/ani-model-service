package model

import "testing"

func TestReadyVersionRequiresVerifiedArtifact(t *testing.T) {
	v := Version{TenantID: "t", ID: "v", ModelID: "m", Version: "v1", Format: "safetensors", Status: "ready"}
	if err := v.DeploymentInput(); err != ErrModelNotReady {
		t.Fatalf("got %v, want ErrModelNotReady", err)
	}
	v.ArtifactProvider, v.ArtifactReference, v.ArtifactSHA256 = "s3", "s3://models/v1", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := v.DeploymentInput(); err != nil {
		t.Fatalf("verified version rejected: %v", err)
	}
}

func TestValidateEngineAcceptsCallerOwnedRuntime(t *testing.T) {
	if err := ValidateEngine("custom-runtime", "/opt/custom-server", []string{"--model", "/models"}); err != nil {
		t.Fatalf("caller-owned runtime rejected: %v", err)
	}
	if err := ValidateEngine("custom-runtime", "", nil); err == nil {
		t.Fatal("incomplete runtime accepted")
	}
}

func TestValidateEngineDefaultsRequiresCompleteTuple(t *testing.T) {
	if err := ValidateEngineDefaults("", "", ""); err != ErrInvalidModelVersion {
		t.Fatalf("expected empty engine defaults rejection, got %v", err)
	}
	if err := ValidateEngineDefaults("vllm", "", "exec vllm serve /models"); err != ErrInvalidModelVersion {
		t.Fatalf("expected partial engine defaults rejection, got %v", err)
	}
	const command = "exec vllm serve /models --served-model-name qwen2.5-1.5b-instruct"
	if err := ValidateEngineDefaults("vllm", "ghcr.io/vllm/vllm-openai:latest", command); err != nil {
		t.Fatalf("exact command should be accepted: %v", err)
	}
}

func TestArtifactValidateRequiresChecksumAndReference(t *testing.T) {
	a := Artifact{TenantID: "t", ID: "a", ModelVersionID: "v", Provider: "storage", Reference: "obj://1", Format: "safetensors", SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	if err := a.Validate(); err != nil {
		t.Fatalf("valid artifact rejected: %v", err)
	}
	a.SHA256 = "bad"
	if err := a.Validate(); err != ErrInvalidModelVersion {
		t.Fatalf("invalid checksum got %v", err)
	}
}

func validBundleImport() BundleImport {
	return BundleImport{
		TenantID: "tenant-a", IdempotencyKey: "bundle-1", Name: "qwen3.5-0.8b",
		Capabilities: []string{"text-generation"}, Version: "master", Format: "safetensors",
		EngineType: "vllm", EngineImage: "registry/vllm:tag", StartupCommand: "vllm serve /models",
		Provider: "modelscope", RepoID: "Qwen/Qwen3.5-0.8B", Revision: "master",
	}
}

func TestBundleImportValidatesCompleteRuntimeInput(t *testing.T) {
	if err := validBundleImport().Validate(); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}
	for name, mutate := range map[string]func(*BundleImport){
		"model name":      func(b *BundleImport) { b.Name = "Qwen 3" },
		"provider":        func(b *BundleImport) { b.Provider = "unknown" },
		"capabilities":    func(b *BundleImport) { b.Capabilities = nil },
		"version":         func(b *BundleImport) { b.Version = "" },
		"format":          func(b *BundleImport) { b.Format = "" },
		"engine":          func(b *BundleImport) { b.EngineType = "" },
		"image":           func(b *BundleImport) { b.EngineImage = "" },
		"startup command": func(b *BundleImport) { b.StartupCommand = "vllm serve\n/models" },
		"idempotency":     func(b *BundleImport) { b.IdempotencyKey = "" },
		"repo":            func(b *BundleImport) { b.RepoID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			bundle := validBundleImport()
			mutate(&bundle)
			if err := bundle.Validate(); err != ErrInvalidBundleImport {
				t.Fatalf("Validate() = %v, want ErrInvalidBundleImport", err)
			}
		})
	}
}
