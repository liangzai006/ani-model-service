package identity

import (
	"context"
	"errors"
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
		return Principal{}, err
	}
	if requestTenant != "" && requestTenant != p.TenantID {
		return Principal{}, ErrTenantMismatch
	}
	return p, nil
}

func HasScope(p Principal, scope string) bool {
	for _, candidate := range p.Scopes {
		if candidate == scope || candidate == "*" {
			return true
		}
	}
	return false
}
