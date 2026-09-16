package server

import (
	"context"
	"errors"

	kratoserrors "github.com/go-kratos/kratos/v3/errors"
	"github.com/go-kratos/kratos/v3/middleware"

	"github.com/zhangzhe-ctrl/ani-model-service/internal/identity"
)

// PrincipalMiddleware binds a trusted identity to the request context before
// business handlers run. The resolver is injected by the composition root so
// this package never derives identity from ordinary request fields or headers.
func PrincipalMiddleware(resolver identity.Resolver) middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			if resolver == nil {
				return next(ctx, req)
			}
			principal, err := resolver.Resolve(ctx)
			if err != nil {
				if kratoserrors.IsUnauthorized(err) {
					return nil, err
				}
				if errors.Is(err, identity.ErrMissingPrincipal) {
					return nil, kratoserrors.Unauthorized("PRINCIPAL_UNAVAILABLE", "trusted principal is unavailable")
				}
				return nil, kratoserrors.Unauthorized("PRINCIPAL_UNAVAILABLE", "trusted principal could not be resolved")
			}
			if !principal.Valid() {
				return nil, kratoserrors.Unauthorized("PRINCIPAL_INVALID", "trusted principal is invalid")
			}
			return next(identity.WithPrincipal(ctx, principal), req)
		}
	}
}
