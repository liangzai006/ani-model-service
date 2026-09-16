package server

import (
	"context"
	"errors"
	"testing"

	kratoserrors "github.com/go-kratos/kratos/v3/errors"

	"github.com/zhangzhe-ctrl/ani-model-service/internal/identity"
)

type testPrincipalResolver struct {
	principal identity.Principal
	err       error
}

func (r testPrincipalResolver) Resolve(context.Context) (identity.Principal, error) {
	return r.principal, r.err
}

func TestPrincipalMiddlewareBindsTrustedPrincipal(t *testing.T) {
	want := identity.Principal{TenantID: "tenant-a", Actor: "user-a", Workload: "gateway", RequestID: "req-1"}
	handler := PrincipalMiddleware(testPrincipalResolver{principal: want})(func(ctx context.Context, _ interface{}) (interface{}, error) {
		got, err := identity.FromContext(ctx)
		if err != nil {
			t.Fatalf("FromContext() error = %v", err)
		}
		if got.TenantID != want.TenantID || got.Actor != want.Actor || got.Workload != want.Workload || got.RequestID != want.RequestID {
			t.Fatalf("principal = %+v, want %+v", got, want)
		}
		return "ok", nil
	})
	got, err := handler(context.Background(), struct{}{})
	if err != nil || got != "ok" {
		t.Fatalf("handler() = (%v, %v), want (ok, nil)", got, err)
	}
}

func TestPrincipalMiddlewareRejectsUntrustedResolution(t *testing.T) {
	handler := PrincipalMiddleware(testPrincipalResolver{err: identity.ErrMissingPrincipal})(func(context.Context, interface{}) (interface{}, error) {
		t.Fatal("next handler should not run")
		return nil, nil
	})
	_, err := handler(context.Background(), struct{}{})
	if !kratoserrors.IsUnauthorized(err) {
		t.Fatalf("error = %v, want unauthorized", err)
	}
}

func TestPrincipalMiddlewareRejectsInvalidPrincipal(t *testing.T) {
	handler := PrincipalMiddleware(testPrincipalResolver{principal: identity.Principal{TenantID: "tenant-a"}})(func(context.Context, interface{}) (interface{}, error) {
		t.Fatal("next handler should not run")
		return nil, nil
	})
	_, err := handler(context.Background(), struct{}{})
	if !kratoserrors.IsUnauthorized(err) {
		t.Fatalf("error = %v, want unauthorized", err)
	}
}

func TestPrincipalMiddlewarePreservesKratosUnauthorized(t *testing.T) {
	want := kratoserrors.Unauthorized("TOKEN_INVALID", "token invalid")
	handler := PrincipalMiddleware(testPrincipalResolver{err: want})(func(context.Context, interface{}) (interface{}, error) {
		t.Fatal("next handler should not run")
		return nil, nil
	})
	_, err := handler(context.Background(), struct{}{})
	if !kratoserrors.IsUnauthorized(err) || !errors.Is(err, want) {
		t.Fatalf("error = %v, want original unauthorized error", err)
	}
}
