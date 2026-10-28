package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	kratosgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	"google.golang.org/grpc"

	conf "github.com/liangzai006/ani-model-service/api/model/v1"
)

// ModelServerTLSConfig is the mandatory mTLS material for the Model gRPC server.
// Client certificates are always required; there is no plaintext or server-only mode.
type ModelServerTLSConfig struct {
	CAFile, CertFile, KeyFile string
}

func (c ModelServerTLSConfig) Validate() error {
	if strings.TrimSpace(c.CertFile) == "" || strings.TrimSpace(c.KeyFile) == "" || strings.TrimSpace(c.CAFile) == "" {
		return errors.New("mTLS requires ANI_MODEL_SERVER_CA_FILE, ANI_MODEL_SERVER_CERT_FILE, and ANI_MODEL_SERVER_KEY_FILE")
	}
	return nil
}

// configuredModelGRPCServer creates a gRPC server that always requires a client
// certificate signed by the configured CA. Callers cannot disable that check.
func configuredModelGRPCServer(c *conf.Server_GRPC, middlewares ...middleware.Middleware) (*kratosgrpc.Server, error) {
	tlsConfig := ModelServerTLSConfig{
		CAFile:   strings.TrimSpace(os.Getenv("ANI_MODEL_SERVER_CA_FILE")),
		CertFile: strings.TrimSpace(os.Getenv("ANI_MODEL_SERVER_CERT_FILE")),
		KeyFile:  strings.TrimSpace(os.Getenv("ANI_MODEL_SERVER_KEY_FILE")),
	}
	if err := tlsConfig.Validate(); err != nil {
		return nil, err
	}

	cert, err := tls.LoadX509KeyPair(tlsConfig.CertFile, tlsConfig.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load Model server certificate: %w", err)
	}
	caPEM, err := os.ReadFile(tlsConfig.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read Model server CA: %w", err)
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("Model server CA bundle has no certificates")
	}

	config := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAs,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if len(verifiedChains) == 0 || len(verifiedChains[0]) == 0 {
				return errors.New("unverified client certificate chain")
			}
			_, err := clientDNSIdentity(verifiedChains[0][0])
			return err
		},
	}

	resolutionTimeout := 300 * time.Second
	if raw := os.Getenv("MODELDEV_MODEL_RESOLUTION_TIMEOUT"); raw != "" {
		var err error
		resolutionTimeout, err = time.ParseDuration(raw)
		if err != nil || resolutionTimeout <= 0 {
			return nil, errors.New("invalid MODELDEV_MODEL_RESOLUTION_TIMEOUT")
		}
	}
	if c == nil || c.Timeout.AsDuration() <= 0 {
		return nil, errors.New("positive model RPC timeout required")
	}
	middlewares = append([]middleware.Middleware{modelMethodBudget(c.Timeout.AsDuration(), resolutionTimeout)}, middlewares...)
	return kratosgrpc.NewServer(
		kratosgrpc.Network(c.Network),
		kratosgrpc.Address(c.Addr),
		kratosgrpc.Timeout(0),
		kratosgrpc.TLSConfig(config),
		kratosgrpc.Middleware(middlewares...),
		kratosgrpc.DisableReflection(),
		kratosgrpc.Options(grpc.MaxRecvMsgSize(conf.MaxFinetuningControlRPCBytes), grpc.MaxSendMsgSize(conf.MaxFinetuningControlRPCBytes)),
	), nil
}

func clientDNSIdentity(cert *x509.Certificate) (string, error) {
	if cert == nil || len(cert.DNSNames) != 1 || len(cert.URIs) != 0 || len(cert.IPAddresses) != 0 || len(cert.EmailAddresses) != 0 {
		return "", errors.New("client certificate must contain exactly one DNS SAN and no URI, IP, or email SANs")
	}
	clientDNS := cert.DNSNames[0]
	if strings.TrimSpace(clientDNS) == "" || strings.ContainsAny(clientDNS, "*:/\\ \t\r\n") {
		return "", fmt.Errorf("invalid client DNS SAN: %q (wildcards not allowed)", clientDNS)
	}
	return clientDNS, nil
}

// A long source read never increases the budget of catalogue writes or queries.
func modelMethodBudget(short, resolution time.Duration) middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			budget := short
			if server, ok := transport.FromServerContext(ctx); ok && server.Operation() == "/model.v1.ModelService/ResolveFinetuningFacts" {
				budget = resolution
			}
			call, cancel := context.WithTimeout(ctx, budget)
			defer cancel()
			return next(call, req)
		}
	}
}
