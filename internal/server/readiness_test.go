package server

import "testing"

func TestReadinessDependencyGate(t *testing.T) {
	r := NewReadiness()
	r.Set(true)
	if !r.Ready() {
		t.Fatal("lifecycle readiness expected")
	}
	r.RequireDependencies()
	if r.Ready() {
		t.Fatal("missing dependencies must be unready")
	}
	r.SetPostgresReady(true)
	r.SetStorageReady(true)
	r.SetWorkerReady(true)
	if !r.Ready() {
		t.Fatal("all dependencies should be ready")
	}
}
