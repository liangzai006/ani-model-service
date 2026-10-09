package main

import (
	"crypto/tls"
	"os"
	"strings"

	inferencev1 "github.com/liangzai006/ani-model-service/api/inference/v1"
	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
	inferencedata "github.com/liangzai006/ani-model-service/internal/data/inference"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func configuredInferenceReferences() (modelbiz.ReferenceChecker, func(), error) {
	close := func() {}
	address := strings.TrimSpace(os.Getenv("ANI_INFERENCE_GRPC_ADDR"))
	if address == "" {
		return nil, close, nil
	}
	var opts []grpc.DialOption
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ANI_ALLOW_DIRECT_ACCESS")), "true") {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		creds := credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, ServerName: os.Getenv("ANI_INFERENCE_GRPC_SERVER_NAME")})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}
	conn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return nil, close, err
	}
	return inferencedata.NewClient(inferencev1.NewModelReferenceServiceClient(conn), ""), func() { _ = conn.Close() }, nil
}
