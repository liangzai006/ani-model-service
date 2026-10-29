package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
	"github.com/liangzai006/ani-model-service/internal/data/postgres"
	storagedata "github.com/liangzai006/ani-model-service/internal/data/storage"
	"github.com/liangzai006/ani-model-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Only the external S3 HTTP boundary is substituted. The real TLS policy,
// principal, service, object verifier and PostgreSQL registration run unchanged.
func trainedArtifactBudgetClient(t *testing.T) (modelv1.ModelServiceClient, string, func(context.Context) (*modelv1.PrepareTrainedArtifactRequest, *modelv1.PrepareTrainedArtifactResponse), <-chan struct{}, <-chan struct{}, *atomic.Int64) {
	t.Helper()
	dsn := os.Getenv("MODEL_DATABASE_URL")
	if dsn == "" {
		t.Skip("MODEL_DATABASE_URL not configured")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tenant := uuid.NewString()
	content := []byte("a fully verified trained artifact through the real TLS registration seam")
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	started, canceled := make(chan struct{}, 8), make(chan struct{}, 8)
	delay := new(atomic.Int64)
	delay.Store(int64(200 * time.Millisecond))
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("location") {
			_, _ = w.Write([]byte(`<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`))
			return
		}
		if (r.Method != http.MethodHead && r.Method != http.MethodGet) || !strings.HasPrefix(r.URL.Path, "/models/"+tenant+"/trained-artifacts/") || !strings.HasSuffix(r.URL.Path, "/model.tar") {
			t.Errorf("unexpected external object request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodGet {
			started <- struct{}{}
			select {
			case <-r.Context().Done():
				canceled <- struct{}{}
				return
			case <-time.After(time.Duration(delay.Load())):
			}
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(content)))
		w.Header().Set("ETag", `"not-the-sha256"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		if r.Method == http.MethodGet {
			_, _ = w.Write(content)
		}
	}))
	t.Cleanup(endpoint.Close)
	storage, err := storagedata.NewMinIOAdapter(strings.TrimPrefix(endpoint.URL, "http://"), "access", "secret", "models", false)
	if err != nil {
		t.Fatal(err)
	}
	app := service.NewModelServiceWithDependencies(nil, nil, nil, storage, nil)
	app.SetTrainedArtifactRegistrar(postgres.NewTrainedArtifactStore(pool))
	caKey, ca, pem := newTestCA(t)
	dir := t.TempDir()
	caFile, certFile, keyFile := filepath.Join(dir, "ca.crt"), filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	if err := os.WriteFile(caFile, pem, 0600); err != nil {
		t.Fatal(err)
	}
	cert, private := signTestCert(t, caKey, ca, "model.ani.internal", nil, x509.ExtKeyUsageServerAuth)
	writeCertPair(t, certFile, keyFile, cert, private)
	t.Setenv("ANI_MODEL_SERVER_CA_FILE", caFile)
	t.Setenv("ANI_MODEL_SERVER_CERT_FILE", certFile)
	t.Setenv("ANI_MODEL_SERVER_KEY_FILE", keyFile)
	// Object reads exceed both unrelated server budgets.
	t.Setenv("MODELDEV_MODEL_RESOLUTION_TIMEOUT", "75ms")
	server, err := configuredModelGRPCServer(&modelv1.Server_GRPC{Network: "tcp", Addr: "127.0.0.1:0", Timeout: durationpb.New(50 * time.Millisecond)}, RPCPolicyMiddleware(ModelServicePolicy()), ModelPrincipalMiddleware())
	if err != nil {
		t.Fatal(err)
	}
	modelv1.RegisterModelServiceServer(server, app)
	addr, err := server.Endpoint()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Start(context.Background()) }()
	t.Cleanup(func() {
		_ = server.Stop(context.Background())
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(pem)
	clientCert, clientKey := signTestCert(t, caKey, ca, modeldevClientIdentity, nil, x509.ExtKeyUsageClientAuth)
	connection, err := grpc.NewClient(addr.Host, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "model.ani.internal", Certificates: []tls.Certificate{{Certificate: [][]byte{clientCert.Raw}, PrivateKey: clientKey}}})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := modelv1.NewModelServiceClient(connection)
	prepare := func(ctx context.Context) (*modelv1.PrepareTrainedArtifactRequest, *modelv1.PrepareTrainedArtifactResponse) {
		t.Helper()
		execution := uuid.NewString()
		request := &modelv1.PrepareTrainedArtifactRequest{TenantId: tenant, ModelName: "budget-" + execution, DisplayName: "TLS budget", Description: "isolated transport regression", Capabilities: []string{"text-generation"}, Version: execution, FileName: "model.tar", Format: "safetensors", SizeBytes: int64(len(content)), ChecksumSha256: digest, ExecutionId: execution, IdempotencyKey: "modeldev:registration:" + execution, EngineType: "vllm", EngineImage: "registry/vllm@sha256:" + strings.Repeat("a", 64), StartupCommand: "vllm serve /models"}
		result, err := client.PrepareTrainedArtifact(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		return request, result
	}
	return client, tenant, prepare, started, canceled, delay
}

func trainedArtifactBudgetContext(tenant string) context.Context {
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-ani-tenant-id", tenant, "x-ani-actor", "modeldev:budget-test", "x-ani-request-id", "request"))
}

func TestModelServerTrainedArtifactVerificationUsesCallerDeadline(t *testing.T) {
	for _, method := range []string{"verified lookup", "complete"} {
		t.Run(method, func(t *testing.T) {
			client, tenant, prepare, _, _, _ := trainedArtifactBudgetClient(t)
			ctx, cancel := context.WithTimeout(trainedArtifactBudgetContext(tenant), 2*time.Second)
			defer cancel()
			request, original := prepare(ctx)
			if method == "complete" {
				result, err := client.CompleteTrainedArtifact(ctx, &modelv1.CompleteTrainedArtifactRequest{TenantId: tenant, RegistrationId: original.RegistrationId, IdempotencyKey: request.IdempotencyKey})
				if err != nil || result.GetModelVersionStatus() != "ready" || result.GetModelVersionId() == "" {
					t.Fatalf("verified completion truncated: result=%v err=%v", result, err)
				}
				return
			}
			lookup := &modelv1.GetTrainedArtifactRegistrationRequest{TenantId: tenant, IdempotencyKey: request.IdempotencyKey, ExpectedExecutionId: request.ExecutionId, VerifyUploadedObject: true}
			result, err := client.GetTrainedArtifactRegistration(ctx, lookup)
			if err != nil || result.GetUploadedObject().GetState() != modelv1.TrainedArtifactObjectObservation_STATE_VERIFIED {
				t.Fatalf("object read truncated: result=%v err=%v", result, err)
			}
			if err := result.ValidateFor(lookup); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestModelServerTrainedArtifactVerificationRequiresCallerDeadline(t *testing.T) {
	for _, method := range []string{"verified lookup", "complete"} {
		t.Run(method, func(t *testing.T) {
			client, tenant, prepare, _, _, _ := trainedArtifactBudgetClient(t)
			ctx, cancel := context.WithTimeout(trainedArtifactBudgetContext(tenant), 2*time.Second)
			defer cancel()
			request, original := prepare(ctx)
			noDeadline := trainedArtifactBudgetContext(tenant)
			var err error
			if method == "complete" {
				_, err = client.CompleteTrainedArtifact(noDeadline, &modelv1.CompleteTrainedArtifactRequest{TenantId: tenant, RegistrationId: original.RegistrationId, IdempotencyKey: request.IdempotencyKey})
			} else {
				_, err = client.GetTrainedArtifactRegistration(noDeadline, &modelv1.GetTrainedArtifactRegistrationRequest{TenantId: tenant, IdempotencyKey: request.IdempotencyKey, ExpectedExecutionId: request.ExecutionId, VerifyUploadedObject: true})
			}
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("unbounded object verification: code=%v err=%v", status.Code(err), err)
			}
			lookup := &modelv1.GetTrainedArtifactRegistrationRequest{TenantId: tenant, IdempotencyKey: request.IdempotencyKey, ExpectedExecutionId: request.ExecutionId}
			observed, err := client.GetTrainedArtifactRegistration(ctx, lookup)
			if err != nil || observed.GetRegistration().GetStatus() != "pending" {
				t.Fatalf("rejected request changed pending registration: result=%v err=%v", observed, err)
			}
		})
	}
}

func TestModelServerTrainedArtifactVerificationHonorsParentCancellation(t *testing.T) {
	for _, method := range []string{"verified lookup", "complete"} {
		for _, stop := range []string{"earlier deadline", "cancel"} {
			t.Run(method+"/"+stop, func(t *testing.T) {
				client, tenant, prepare, started, canceled, delay := trainedArtifactBudgetClient(t)
				setup, setupCancel := context.WithTimeout(trainedArtifactBudgetContext(tenant), 2*time.Second)
				request, original := prepare(setup)
				setupCancel()
				delay.Store(int64(5 * time.Second))
				budget := 2 * time.Second
				expected := codes.Canceled
				if stop == "earlier deadline" {
					budget = 200 * time.Millisecond
					expected = codes.DeadlineExceeded
				}
				ctx, cancel := context.WithTimeout(trainedArtifactBudgetContext(tenant), budget)
				defer cancel()
				result := make(chan error, 1)
				go func() {
					var err error
					if method == "complete" {
						_, err = client.CompleteTrainedArtifact(ctx, &modelv1.CompleteTrainedArtifactRequest{TenantId: tenant, RegistrationId: original.RegistrationId, IdempotencyKey: request.IdempotencyKey})
					} else {
						_, err = client.GetTrainedArtifactRegistration(ctx, &modelv1.GetTrainedArtifactRegistrationRequest{TenantId: tenant, IdempotencyKey: request.IdempotencyKey, ExpectedExecutionId: request.ExecutionId, VerifyUploadedObject: true})
					}
					result <- err
				}()
				select {
				case <-started:
				case err := <-result:
					t.Fatalf("request did not reach external object read: %v", err)
				case <-time.After(time.Second):
					t.Fatal("external object read did not start")
				}
				if stop == "cancel" {
					cancel()
				}
				select {
				case err := <-result:
					if status.Code(err) != expected {
						t.Fatalf("parent stop ignored: code=%v want=%v err=%v", status.Code(err), expected, err)
					}
				case <-time.After(time.Second):
					t.Fatal("RPC outlived parent stop")
				}
				select {
				case <-canceled:
				case <-time.After(time.Second):
					t.Fatal("external object read outlived parent stop")
				}
				readCtx, readCancel := context.WithTimeout(trainedArtifactBudgetContext(tenant), time.Second)
				defer readCancel()
				observed, err := client.GetTrainedArtifactRegistration(readCtx, &modelv1.GetTrainedArtifactRegistrationRequest{TenantId: tenant, IdempotencyKey: request.IdempotencyKey, ExpectedExecutionId: request.ExecutionId})
				if err != nil || observed.GetRegistration().GetStatus() != "pending" {
					t.Fatalf("stopped verification changed pending registration: result=%v err=%v", observed, err)
				}
			})
		}
	}
}
