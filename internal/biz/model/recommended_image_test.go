package model

import "testing"

func TestRecommendedImageRequiresPairAndKnownScope(t *testing.T) {
	for _, tc := range []struct {
		name, id, scope string
		valid           bool
	}{
		{"legacy", "", "", true},
		{"platform", "image-a", "platform", true},
		{"tenant", "image-a", "tenant", true},
		{"missing id", "", "tenant", false},
		{"missing scope", "image-a", "", false},
		{"invalid scope", "image-a", "public", false},
		{"blank id", " ", "tenant", false},
		{"padded id", " image-a", "tenant", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidateRecommendedImage(tc.id, tc.scope) == nil; got != tc.valid {
				t.Fatalf("valid=%v, want %v", got, tc.valid)
			}
			v := Version{TenantID: "tenant", ID: "version", ModelID: "model", Version: "v1", Format: "gguf", RecommendedImageID: tc.id, RecommendedImageScope: tc.scope}
			if got := v.Validate() == nil; got != tc.valid {
				t.Fatalf("version valid=%v, want %v", got, tc.valid)
			}
		})
	}
}
