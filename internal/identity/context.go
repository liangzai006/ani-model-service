package identity

import (
	"context"
	"errors"
	"os"
	"strings"
)

var (
	ErrMissingPrincipal = errors.New("trusted principal is required")
	ErrTenantMismatch   = errors.New("request tenant does not match trusted principal")
)

type Principal struct {
	TenantID  string
	Actor     string
	Workload  string
	RequestID string
	Scopes    []string
}

// Resolver obtains a trusted principal from the transport context. Concrete
// implementations belong to the authentication boundary (Gateway or IAM).
type Resolver interface {
	Resolve(context.Context) (Principal, error)
}

func (p Principal) Valid() bool { return p.TenantID != "" && p.Actor != "" && p.Workload != "" }

type contextKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}

func FromContext(ctx context.Context) (Principal, error) {
	p, ok := ctx.Value(contextKey{}).(Principal)
	if !ok || !p.Valid() {
		return Principal{}, ErrMissingPrincipal
	}
	return p, nil
}

func RequireTenant(ctx context.Context, requestTenant string) (Principal, error) {
	p, err := FromContext(ctx)
	if err != nil {
		if errors.Is(err, ErrMissingPrincipal) && requestTenant != "" {
			// SECURITY: Authentication bypass for isolated deployments ONLY.
			// Set ANI_ALLOW_DIRECT_ACCESS=true explicitly to enable this mode.
			// DO NOT enable in production with external traffic.
			if !isDirectAccessAllowed() {
				return Principal{}, ErrMissingPrincipal
			}
			// The isolated validation deployment has no IAM boundary. Keep the
			// request tenant as the local scope and mark the caller explicitly.
			return Principal{TenantID: requestTenant, Actor: "direct", Workload: "direct", RequestID: "direct"}, nil
		}
		return Principal{}, err
	}
	if requestTenant != "" && requestTenant != p.TenantID {
		return Principal{}, ErrTenantMismatch
	}
	return p, nil
}

// isDirectAccessAllowed checks if the authentication bypass is explicitly enabled.
// This should ONLY be true in isolated development/testing environments.
func isDirectAccessAllowed() bool {
	value := os.Getenv("ANI_ALLOW_DIRECT_ACCESS")
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func HasScope(p Principal, scope string) bool {
	for _, candidate := range p.Scopes {
		if candidate == scope || candidate == "*" {
			return true
		}
	}
	return false
}
