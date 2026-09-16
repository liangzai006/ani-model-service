package idempotency

import "testing"

func TestValidateAndFingerprint(t *testing.T) {
	if ValidateKey("") == nil || ValidateKey("bad key") == nil {
		t.Fatal("invalid keys accepted")
	}
	if ValidateKey("model:create:1") != nil {
		t.Fatal("valid key rejected")
	}
	a := Fingerprint([]byte("request"))
	if !ReplayMatches(a, a) || ReplayMatches(a, Fingerprint([]byte("other"))) {
		t.Fatal("fingerprint replay mismatch")
	}
}
