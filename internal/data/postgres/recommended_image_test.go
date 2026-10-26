package postgres

import (
	"encoding/json"
	"testing"

	"github.com/liangzai006/ani-model-service/internal/biz/idempotency"
	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
)

func TestVersionRecommendedImageFingerprintPreservesLegacyAndDetectsSelection(t *testing.T) {
	v := modelbiz.Version{ModelID: "model", Version: "v1", Format: "gguf", EngineType: "llama.cpp", EngineImage: "registry/llama:old", StartupCommand: "serve /models"}
	legacy, _ := json.Marshal(struct {
		ModelID, Version, Format, ArtifactReference, ArtifactSHA256, EngineType, EngineImage, StartupCommand string
		StartupArgs                                                                                          []string
	}{v.ModelID, v.Version, v.Format, v.ArtifactReference, v.ArtifactSHA256, v.EngineType, v.EngineImage, v.StartupCommand, v.StartupArgs})
	if versionFingerprint(v) != idempotency.Fingerprint(legacy) {
		t.Fatal("empty recommendation changed the historical fingerprint")
	}
	v.RecommendedImageID, v.RecommendedImageScope = "image-a", "platform"
	selected := versionFingerprint(v)
	if selected == idempotency.Fingerprint(legacy) {
		t.Fatal("selection was omitted from fingerprint")
	}
	v.RecommendedImageID = "image-b"
	if versionFingerprint(v) == selected {
		t.Fatal("changed image id retained fingerprint")
	}
	v.RecommendedImageID, v.RecommendedImageScope = "image-a", "tenant"
	if versionFingerprint(v) == selected {
		t.Fatal("changed image scope retained fingerprint")
	}
}

func TestBundleRecommendedImageFingerprintPreservesLegacyAndDetectsSelection(t *testing.T) {
	in := bundleImportFixture("tenant", "key")
	// Freeze the old JSON field order and names: adding optional fields must
	// retain the exact fingerprint used by stored pre-upgrade requests.
	legacy, _ := json.Marshal(struct {
		TenantID, IdempotencyKey                        string
		Name, DisplayName, Description, ExternalModelID string
		Capabilities                                    []string
		Version, Format, EngineType, EngineImage        string
		StartupCommand                                  string
		Provider, RepoID, Revision                      string
	}{in.TenantID, in.IdempotencyKey, in.Name, in.DisplayName, in.Description, in.ExternalModelID, in.Capabilities, in.Version, in.Format, in.EngineType, in.EngineImage, in.StartupCommand, in.Provider, in.RepoID, in.Revision})
	first, err := bundleFingerprint(in)
	if err != nil || first != idempotency.Fingerprint(legacy) {
		t.Fatalf("legacy fingerprint changed: %v", err)
	}
	in.RecommendedImageID, in.RecommendedImageScope = "image-a", "tenant"
	selected, _ := bundleFingerprint(in)
	if selected == first {
		t.Fatal("selection was omitted from bundle fingerprint")
	}
	in.RecommendedImageScope = "platform"
	changed, _ := bundleFingerprint(in)
	if changed == selected {
		t.Fatal("changed scope retained bundle fingerprint")
	}
}
