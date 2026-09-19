package main

import (
	"context"
	"log/slog"
	"time"

	kratos "github.com/go-kratos/kratos/v3"
	kratosgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	kratoshttp "github.com/go-kratos/kratos/v3/transport/http"

	conf "github.com/liangzai006/ani-model-service/api/model/v1"
	"github.com/liangzai006/ani-model-service/internal/server"
	"github.com/liangzai006/ani-model-service/internal/service"
)

func buildApp(bc *conf.Bootstrap, logger *slog.Logger) (*kratos.App, error) {
	return buildAppWithModelService(bc, logger, service.NewModelService(nil), false, false, false)
}

func buildAppWithModelService(bc *conf.Bootstrap, logger *slog.Logger, modelService *service.ModelService, dependencyGated bool, postgresReady, storageReady bool, workerRunners ...server.WorkerRunner) (*kratos.App, error) {
	if err := bc.Validate(); err != nil {
		return nil, err
	}
	readiness := server.NewReadiness()
	if dependencyGated {
		readiness.RequireDependencies()
		readiness.SetPostgresReady(postgresReady)
		readiness.SetStorageReady(storageReady)
	}
	observability, err := server.NewObservability(Name, Version, readiness)
	if err != nil {
		return nil, err
	}
	// Identity is supplied by the trusted ingress/IAM boundary. The service
	// deliberately has no local development principal fallback.
	middlewares := observability.ServerMiddleware(logger)
	grpcServer := server.NewGRPCServer(bc.Server.Grpc, middlewares...)
	conf.RegisterModelServiceServer(grpcServer, modelService)
	adminServer := server.NewAdminServer(bc.Server.Admin, readiness, observability.Gatherer(), middlewares...)
	var supervisor *server.WorkerSupervisor
	if len(workerRunners) > 0 && workerRunners[0] != nil {
		supervisor = server.NewWorkerSupervisor(workerRunners[0], readiness)
	}
	return newApp(logger, grpcServer, adminServer, readiness, observability, supervisor, bc.Server.ShutdownTimeout.AsDuration()), nil
}

func newApp(
	logger *slog.Logger,
	grpcServer *kratosgrpc.Server,
	adminServer *kratoshttp.Server,
	readiness *server.Readiness,
	observability *server.Observability,
	workerSupervisor *server.WorkerSupervisor,
	stopTimeout time.Duration,
) *kratos.App {
	return kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Logger(logger),
		kratos.Server(grpcServer, adminServer),
		kratos.AfterStart(func(ctx context.Context) error {
			if workerSupervisor != nil {
				if err := workerSupervisor.Start(ctx); err != nil {
					return err
				}
			}
			readiness.Set(true)
			return nil
		}),
		kratos.BeforeStop(func(ctx context.Context) error {
			readiness.Set(false)
			if workerSupervisor != nil {
				return workerSupervisor.Stop(ctx)
			}
			return nil
		}),
		kratos.AfterStop(observability.Shutdown),
		kratos.StopTimeout(stopTimeout),
	)
}
