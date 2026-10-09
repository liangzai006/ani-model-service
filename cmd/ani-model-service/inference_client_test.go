package main

import (
	"context"
	"net"
	"testing"

	inferencev1 "github.com/liangzai006/ani-model-service/api/inference/v1"
	"google.golang.org/grpc"
)

type directReferenceServer struct {
	inferencev1.UnimplementedModelReferenceServiceServer
}

func (directReferenceServer) CheckModelVersionReferences(context.Context, *inferencev1.CheckModelVersionReferencesRequest) (*inferencev1.CheckModelVersionReferencesResponse, error) {
	return &inferencev1.CheckModelVersionReferencesResponse{}, nil
}

func TestConfiguredInferenceReferencesUsesDirectAccessTransport(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	inferencev1.RegisterModelReferenceServiceServer(server, directReferenceServer{})
	go server.Serve(listener)
	defer server.Stop()

	t.Setenv("ANI_INFERENCE_GRPC_ADDR", listener.Addr().String())
	t.Setenv("ANI_ALLOW_DIRECT_ACCESS", "true")
	references, closeReferences, err := configuredInferenceReferences()
	if err != nil {
		t.Fatal(err)
	}
	defer closeReferences()
	if references == nil {
		t.Fatal("configured reference checker is nil")
	}
	if _, err := references.HasActiveVersionReferences(context.Background(), "tenant-a", []string{"22222222-2222-4222-8222-222222222222"}); err != nil {
		t.Fatalf("direct reference check failed: %v", err)
	}
}

func TestConfiguredInferenceReferencesKeepsTLSWhenDirectAccessDisabled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	inferencev1.RegisterModelReferenceServiceServer(server, directReferenceServer{})
	go server.Serve(listener)
	defer server.Stop()

	t.Setenv("ANI_INFERENCE_GRPC_ADDR", listener.Addr().String())
	t.Setenv("ANI_ALLOW_DIRECT_ACCESS", "false")
	references, closeReferences, err := configuredInferenceReferences()
	if err != nil {
		t.Fatal(err)
	}
	defer closeReferences()
	_, err = references.HasActiveVersionReferences(context.Background(), "tenant-a", []string{"22222222-2222-4222-8222-222222222222"})
	if err == nil {
		t.Fatal("plaintext server unexpectedly accepted TLS-disabled direct access")
	}
}
