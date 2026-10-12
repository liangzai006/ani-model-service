package main

import "testing"

func TestConfiguredInferenceReferencesDisabledWithoutAddress(t *testing.T) {
	t.Setenv("ANI_INFERENCE_GRPC_ADDR", "")
	references, closeReferences, err := configuredInferenceReferences()
	if err != nil {
		t.Fatal(err)
	}
	defer closeReferences()
	if references != nil {
		t.Fatal("inference reference checker should be disabled without an address")
	}
}

func TestConfiguredInferenceReferencesRequiresTLS(t *testing.T) {
	t.Setenv("ANI_INFERENCE_GRPC_ADDR", "inference.ani-system.svc.cluster.local:19090")
	t.Setenv("ANI_INFERENCE_CA_FILE", "")
	t.Setenv("ANI_INFERENCE_CERT_FILE", "")
	t.Setenv("ANI_INFERENCE_KEY_FILE", "")
	t.Setenv("ANI_INFERENCE_SERVER_NAME", "")
	_, closeReferences, err := configuredInferenceReferences()
	defer closeReferences()
	if err == nil {
		t.Fatal("expected mTLS configuration error")
	}
}
