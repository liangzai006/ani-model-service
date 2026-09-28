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

func TestRequireTenantAllowsDirectRequestTenant(t *testing.T) {
	// SECURITY: This test validates that direct access requires explicit opt-in
	t.Setenv("ANI_ALLOW_DIRECT_ACCESS", "true")
	got, err := RequireTenant(context.Background(), "tenant-a")
	if err != nil || got.TenantID != "tenant-a" || got.Actor != "direct" || got.Workload != "direct" || got.RequestID == "" {
		t.Fatalf("direct principal = %#v, err = %v", got, err)
	}
}

func TestRequireTenantDeniesDirectAccessByDefault(t *testing.T) {
	// SECURITY: Without ANI_ALLOW_DIRECT_ACCESS, direct mode should fail
	_, err := RequireTenant(context.Background(), "tenant-123")
	if !errors.Is(err, ErrMissingPrincipal) {
		t.Fatalf("expected ErrMissingPrincipal, got %v", err)
	}
}
