package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
	"github.com/liangzai006/ani-model-service/internal/service"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Run this opt-in receiver from a compiled test binary while Governance's real
// ModelClient calls the production service with invalid business input. It
// verifies mTLS, method authorization and principal binding without a database.
func TestModelIdentityInteropReceiver(t *testing.T) {
	dir := os.Getenv("MODEL_IDENTITY_INTEROP_DIR")
	if dir == "" {
		t.Skip("MODEL_IDENTITY_INTEROP_DIR not configured; requires a Governance client process")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("interop directory must already exist: %s", dir)
	}
	caKey, caCert, caPEM := newTestCA(t)
	certDir := t.TempDir()
	caPath := filepath.Join(certDir, "ca.crt")
	serverCertPath, serverKeyPath := filepath.Join(certDir, "server.crt"), filepath.Join(certDir, "server.key")
	clientCertPath, clientKeyPath := filepath.Join(certDir, "governance.crt"), filepath.Join(certDir, "governance.key")
	if err := os.WriteFile(caPath, caPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	const serverName = "ani-model.ani.internal"
	serverCert, serverKey := signTestCert(t, caKey, caCert, serverName, nil, x509.ExtKeyUsageServerAuth)
	writeCertPair(t, serverCertPath, serverKeyPath, serverCert, serverKey)
	clientCert, clientKey := signTestCert(t, caKey, caCert, "ani-governance", nil, x509.ExtKeyUsageClientAuth)
	writeCertPair(t, clientCertPath, clientKeyPath, clientCert, clientKey)
	t.Setenv("ANI_MODEL_SERVER_CA_FILE", caPath)
	t.Setenv("ANI_MODEL_SERVER_CERT_FILE", serverCertPath)
	t.Setenv("ANI_MODEL_SERVER_KEY_FILE", serverKeyPath)
	server, err := configuredModelGRPCServer(&modelv1.Server_GRPC{
		Network: "tcp", Addr: "127.0.0.1:0", Timeout: durationpb.New(time.Second),
	}, RPCPolicyMiddleware(ModelServicePolicy()), ModelPrincipalMiddleware())
	if err != nil {
		t.Fatal(err)
	}
	modelv1.RegisterModelServiceServer(server, service.NewModelService(nil))
	endpoint, err := server.Endpoint()
	if err != nil {
		t.Fatal(err)
	}
	runResult := make(chan error, 1)
	go func() { runResult <- server.Start(context.Background()) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := server.Stop(ctx); err != nil {
			t.Error(err)
		}
		if err := <-runResult; err != nil {
			t.Error(err)
		}
	})
	config, err := json.Marshal(map[string]string{
		"endpoint": endpoint.Host, "ca_file": caPath, "cert_file": clientCertPath,
		"key_file": clientKeyPath, "server_name": serverName,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "model.json"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("production Model receiver listening at %s", endpoint.Host)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	select {
	case <-ctx.Done():
	case <-time.After(60 * time.Second):
	}
}
