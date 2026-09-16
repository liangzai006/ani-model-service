package audit

import "testing"

func TestEventRequiresTrustedContextAndChange(t *testing.T) {
	e := Event{TenantID: "t", Actor: "a", Workload: "w", RequestID: "r", Action: "model.create"}
	if e.Validate() == nil {
		t.Fatal("expected missing state rejection")
	}
	e.AfterState = "pending"
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
}
