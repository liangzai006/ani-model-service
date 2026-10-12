package main

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// RPCPolicyMiddleware enforces method-level authorization based on client certificate identity.
// This implements deny-by-default: only explicitly allowed (client identity, RPC method) pairs are permitted.
func RPCPolicyMiddleware(policy RPCPolicy) middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return nil, status.Error(codes.Internal, "transport context unavailable")
			}
			method := tr.Operation()

			clientIdentity, err := extractClientIdentity(ctx)
			if err != nil {
				return nil, status.Errorf(codes.Unauthenticated, "client identity unavailable: %v", err)
			}
			// gRPC health is not a business method. The certificate is already
			// required by the server TLS config; kubelet uses HTTP probes instead.
			if method == "/grpc.health.v1.Health/Check" || method == "/grpc.health.v1.Health/Watch" {
				return next(ctx, req)
			}

			if !policy.IsAllowed(clientIdentity, method) {
				return nil, status.Errorf(codes.PermissionDenied,
					"client %q not authorized for method %q", clientIdentity, method)
			}

			return next(ctx, req)
		}
	}
}

// extractClientIdentity retrieves the DNS SAN from the client's mTLS certificate.
func extractClientIdentity(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("no peer info")
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", fmt.Errorf("not a TLS connection")
	}

	if len(tlsInfo.State.PeerCertificates) == 0 {
		return "", fmt.Errorf("no client certificate")
	}

	cert := tlsInfo.State.PeerCertificates[0]
	if len(cert.DNSNames) != 1 {
		return "", fmt.Errorf("client certificate must have exactly one DNS SAN, found %d", len(cert.DNSNames))
	}

	return cert.DNSNames[0], nil
}

// RPCPolicy defines which client identities can call which RPC methods.
type RPCPolicy interface {
	IsAllowed(clientIdentity, method string) bool
}

// StaticRPCPolicy is a simple map-based policy implementation.
type StaticRPCPolicy map[string][]string

func (p StaticRPCPolicy) IsAllowed(clientIdentity, method string) bool {
	allowedMethods, ok := p[clientIdentity]
	if !ok {
		return false
	}
	for _, m := range allowedMethods {
		if m == method {
			return true
		}
	}
	return false
}

// ModelServicePolicy defines the RPC authorization policy for Model Service.
// Only Inference Service is allowed to call read-only model query methods.
func ModelServicePolicy() RPCPolicy {
	policy := StaticRPCPolicy{
		"ani-inference.ani.internal": {
			"/model.v1.ModelService/GetModel",
			"/model.v1.ModelService/GetModelVersion",
			"/model.v1.ModelService/GetModelDownloadURL",
		},
	}

	return &ValidatedPolicy{
		underlying: policy,
		expectedIdentities: map[string]string{
			"ani-inference.ani.internal": "Inference Service",
		},
	}
}

// ValidatedPolicy wraps a policy with explicit service identity validation.
type ValidatedPolicy struct {
	underlying         RPCPolicy
	expectedIdentities map[string]string
}

func (v *ValidatedPolicy) IsAllowed(clientIdentity, method string) bool {
	if _, expected := v.expectedIdentities[clientIdentity]; !expected {
		return false
	}
	return v.underlying.IsAllowed(clientIdentity, method)
}
