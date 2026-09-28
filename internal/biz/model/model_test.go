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
