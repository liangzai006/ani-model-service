package identity

import (
	"context"
	"errors"
	"testing"
)

func TestRequireTenantUsesTrustedContext(t *testing.T) {
	p := Principal{TenantID: "tenant-a", Actor: "user-a", Workload: "ani-inference-service"}
	ctx := WithPrincipal(context.Background(), p)
	if _, err := RequireTenant(ctx, "tenant-b"); !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("got %v, want tenant mismatch", err)
	}
	got, err := RequireTenant(ctx, "tenant-a")
	if err != nil || got.Actor != p.Actor {
		t.Fatalf("trusted principal lost: %#v, %v", got, err)
	}
}

func TestRequireTenantRejectsMissingPrincipal(t *testing.T) {
	if _, err := RequireTenant(context.Background(), "tenant-a"); !errors.Is(err, ErrMissingPrincipal) {
		t.Fatalf("got %v, want missing principal", err)
	}
}
