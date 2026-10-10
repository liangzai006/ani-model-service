package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-kratos/kratos/v3/middleware"
	kratosgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	"google.golang.org/grpc/credentials"

	conf "github.com/liangzai006/ani-model-service/api/model/v1"
)

// ModelServerTLSConfig holds the mTLS configuration for the Model gRPC server.
// When client certificate verification is enabled, only clients with valid
// certificates signed by the configured CA can connect.
type ModelServerTLSConfig struct {
	CAFile, CertFile, KeyFile string
	RequireClientCert         bool
}

func (c ModelServerTLSConfig) IsConfigured() bool {
	return c.CertFile != "" || c.KeyFile != ""
}

func (c ModelServerTLSConfig) Validate() error {
	if strings.TrimSpace(c.CertFile) == "" || strings.TrimSpace(c.KeyFile) == "" {
		return errors.New("server TLS requires cert and key files")
	}
	if c.RequireClientCert && strings.TrimSpace(c.CAFile) == "" {
		return errors.New("client certificate verification requires CA file")
	}
	return nil
}

// configuredModelGRPCServer creates a gRPC server with optional mTLS support.
// When TLS environment variables are not set, it creates a plaintext server
// for local development. Production deployments should always configure mTLS.
func configuredModelGRPCServer(c *conf.Server_GRPC, middlewares ...middleware.Middleware) (*kratosgrpc.Server, error) {
	tlsConfig := ModelServerTLSConfig{
		CAFile:            strings.TrimSpace(os.Getenv("ANI_MODEL_SERVER_CA_FILE")),
		CertFile:          strings.TrimSpace(os.Getenv("ANI_MODEL_SERVER_CERT_FILE")),
		KeyFile:           strings.TrimSpace(os.Getenv("ANI_MODEL_SERVER_KEY_FILE")),
		RequireClientCert: strings.EqualFold(strings.TrimSpace(os.Getenv("ANI_MODEL_REQUIRE_CLIENT_CERT")), "true"),
	}

	// If no TLS config, create plaintext server (development only)
	if !tlsConfig.IsConfigured() {
		return kratosgrpc.NewServer(
			kratosgrpc.Network(c.Network),
			kratosgrpc.Address(c.Addr),
			kratosgrpc.Timeout(c.Timeout.AsDuration()),
			kratosgrpc.Middleware(middlewares...),
			kratosgrpc.DisableReflection(),
		), nil
	}

	// Production: configure mTLS
	if err := tlsConfig.Validate(); err != nil {
		return nil, fmt.Errorf("model server TLS config: %w", err)
	}

	cert, err := tls.LoadX509KeyPair(tlsConfig.CertFile, tlsConfig.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load Model server certificate: %w", err)
	}

	config := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
	}

	// If client cert verification is required
	if tlsConfig.RequireClientCert {
		caPEM, err := os.ReadFile(tlsConfig.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read Model server CA: %w", err)
		}

		clientCAs := x509.NewCertPool()
		if !clientCAs.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("Model server CA bundle has no certificates")
		}

		config.ClientAuth = tls.RequireAndVerifyClientCert
		config.ClientCAs = clientCAs
	}

	return kratosgrpc.NewServer(
		kratosgrpc.Network(c.Network),
		kratosgrpc.Address(c.Addr),
		kratosgrpc.Timeout(c.Timeout.AsDuration()),
		kratosgrpc.TLSConfig(config),
		kratosgrpc.Middleware(middlewares...),
		kratosgrpc.DisableReflection(),
	), nil
}
