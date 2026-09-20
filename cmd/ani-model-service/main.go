package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-kratos/kratos/contrib/otel/v3/tracing"
	"github.com/go-kratos/kratos/v3/config"
	"github.com/go-kratos/kratos/v3/config/env"
	"github.com/go-kratos/kratos/v3/config/file"
	"github.com/go-kratos/kratos/v3/log"
	"go.uber.org/automaxprocs/maxprocs"

	"github.com/jackc/pgx/v5/pgxpool"
	conf "github.com/liangzai006/ani-model-service/api/model/v1"
	bizstorage "github.com/liangzai006/ani-model-service/internal/biz/storage"
	"github.com/liangzai006/ani-model-service/internal/data/importer"
	kubedata "github.com/liangzai006/ani-model-service/internal/data/kubernetes"
	"github.com/liangzai006/ani-model-service/internal/data/postgres"
	storagedata "github.com/liangzai006/ani-model-service/internal/data/storage"
	"github.com/liangzai006/ani-model-service/internal/server"
	"github.com/liangzai006/ani-model-service/internal/service"
	"github.com/liangzai006/ani-model-service/internal/worker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Name and Version can be overridden with -ldflags at build time.
var (
	Name     = "ani-model-service"
	Version  = "dev"
	flagconf string
	id, _    = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "configs", "config path, for example -conf configs/config.yaml")
}

func main() {
	flag.Parse()
	logger := newRuntimeLogger(os.Stdout)
	log.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("service terminated", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	undoMaxProcs, err := maxprocs.Set(maxprocs.Logger(func(format string, args ...interface{}) {
		log.Info("runtime CPU quota", "detail", fmt.Sprintf(format, args...))
	}))
	if err != nil {
		return fmt.Errorf("configure runtime CPU quota: %w", err)
	}
	defer undoMaxProcs()

	c := config.New(config.WithSource(
		file.NewSource(flagconf),
		env.NewSource("ANI"),
	))
	defer c.Close()
	if err := c.Load(); err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		return fmt.Errorf("scan config: %w", err)
	}
	modelService := service.NewModelService(nil)
	var importWorker *worker.Worker
	var storagePort bizstorage.Port
	var storageConn *grpc.ClientConn
	storageReady := false
	minioEndpoint := strings.TrimSpace(os.Getenv("ANI_MINIO_ENDPOINT"))
	minioSecure := os.Getenv("ANI_MINIO_SECURE") == "true"
	minioTenantBuckets := strings.EqualFold(strings.TrimSpace(os.Getenv("ANI_MINIO_TENANT_BUCKETS")), "true")
	minioBucket := firstEnv("ANI_MINIO_BUCKET", "ani-models")
	kubernetesImportMode := strings.EqualFold(strings.TrimSpace(os.Getenv("ANI_IMPORT_EXECUTION_MODE")), "kubernetes")
	grpcEndpoint := strings.TrimSpace(os.Getenv("ANI_STORAGE_GRPC_ADDR"))
	if minioEndpoint != "" && grpcEndpoint != "" {
		return fmt.Errorf("configure only one of ANI_MINIO_ENDPOINT and ANI_STORAGE_GRPC_ADDR")
	}
	if minioEndpoint != "" {
		var minioStorage *storagedata.MinIOAdapter
		var createErr error
		if minioTenantBuckets {
			minioStorage, createErr = storagedata.NewMinIOTenantBucketAdapter(minioEndpoint, os.Getenv("ANI_MINIO_ACCESS_KEY"), os.Getenv("ANI_MINIO_SECRET_KEY"), minioSecure)
		} else {
			minioStorage, createErr = storagedata.NewMinIOAdapter(minioEndpoint, os.Getenv("ANI_MINIO_ACCESS_KEY"), os.Getenv("ANI_MINIO_SECRET_KEY"), minioBucket, minioSecure)
		}
		err = createErr
		if err != nil {
			return fmt.Errorf("configure MinIO Storage: %w", err)
		}
		if !minioTenantBuckets && !kubernetesImportMode {
			checkCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = minioStorage.CheckBucket(checkCtx)
			cancel()
			if err != nil {
				return fmt.Errorf("check MinIO bucket: %w", err)
			}
		}
		storagePort = minioStorage
		storageReady = true
	}
	if grpcEndpoint != "" {
		dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		opts := []grpc.DialOption{
			grpc.WithBlock(),
			grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, ServerName: os.Getenv("ANI_STORAGE_GRPC_SERVER_NAME")})),
		}
		storagePort, storageConn, err = storagedata.DialGRPC(dialCtx, grpcEndpoint, opts...)
		cancel()
		if err != nil {
			return fmt.Errorf("connect Storage gRPC: %w", err)
		}
		defer storageConn.Close()
		storageReady = true
	}
	var pool *pgxpool.Pool
	postgresReady := false
	if dsn := os.Getenv("ANI_DATABASE_DSN"); dsn != "" {
		pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pool, err = pgxpool.New(pingCtx, dsn)
		if err == nil {
			err = pool.Ping(pingCtx)
		}
		cancel()
		if err != nil {
			if pool != nil {
				pool.Close()
			}
			return fmt.Errorf("connect PostgreSQL: %w", err)
		}
		defer pool.Close()
		postgresReady = true
		modelStore := postgres.NewModelStore(pool)
		versionStore := postgres.NewVersionStore(pool)
		workStore := postgres.NewWorkStore(pool)
		modelService = service.NewModelServiceWithDependencies(versionStore, versionStore, postgres.NewCatalogStore(modelStore), storagePort, workStore)
		modelService.SetImportTaskReader(workStore)
		modelService.SetImportTaskRetrier(workStore)
		modelService.SetArtifactStore(postgres.NewArtifactStore(pool))
		modelService.SetAuditStore(postgres.NewAuditStore(pool))
		if storagePort != nil {
			owner := strings.TrimSpace(os.Getenv("ANI_IMPORT_WORKER_OWNER"))
			if owner == "" {
				owner = id
			}
			providers := importer.Registry{
				"huggingface": importer.NewHuggingFaceAdapter(os.Getenv("ANI_HUGGINGFACE_BASE_URL"), nil),
				"modelscope":  importer.NewModelScopeAdapter(os.Getenv("ANI_MODELSCOPE_BASE_URL"), nil),
			}
			artifactStore := postgres.NewArtifactStore(pool)
			var executor worker.Executor = worker.ImportExecutor{
				Providers: providers,
				Storage:   storagePort,
				Artifacts: artifactStore,
				Checksums: versionStore,
				Logger:    logger,
			}
			if kubernetesImportMode {
				if minioEndpoint == "" {
					return fmt.Errorf("ANI_IMPORT_EXECUTION_MODE=kubernetes requires direct MinIO configuration via ANI_MINIO_ENDPOINT")
				}
				kubeConfig, configErr := rest.InClusterConfig()
				if configErr != nil {
					return fmt.Errorf("configure Kubernetes import Job: %w", configErr)
				}
				kubeClient, clientErr := k8sclient.NewForConfig(kubeConfig)
				if clientErr != nil {
					return fmt.Errorf("create Kubernetes import client: %w", clientErr)
				}
				executor = worker.KubernetesImportExecutor{
					Jobs:               kubedata.NewImportJobClient(kubeClient),
					Storage:            storagePort,
					Artifacts:          artifactStore,
					Checksums:          versionStore,
					Providers:          providers,
					Namespace:          firstEnv("ANI_IMPORT_KUBERNETES_NAMESPACE", "ani-model-inference"),
					Image:              strings.TrimSpace(os.Getenv("ANI_IMPORT_JOB_IMAGE")),
					ServiceAccount:     strings.TrimSpace(os.Getenv("ANI_IMPORT_JOB_SERVICE_ACCOUNT")),
					MinIOSecretName:    strings.TrimSpace(os.Getenv("ANI_IMPORT_MINIO_SECRET")),
					ProviderSecretName: strings.TrimSpace(os.Getenv("ANI_IMPORT_PROVIDER_SECRET")),
					MinIOEndpoint:      minioEndpoint,
					MinIOBucket:        minioBucket,
					MinIOTenantBuckets: minioTenantBuckets,
					MinIOSecure:        minioSecure,
					StorageClass:       strings.TrimSpace(os.Getenv("ANI_IMPORT_STORAGE_CLASS")),
					StorageSize:        strings.TrimSpace(os.Getenv("ANI_IMPORT_STORAGE_SIZE")),
					PollInterval:       time.Second,
				}
				if strings.TrimSpace(os.Getenv("ANI_IMPORT_JOB_IMAGE")) == "" {
					return fmt.Errorf("ANI_IMPORT_JOB_IMAGE is required in Kubernetes import mode")
				}
				if strings.TrimSpace(os.Getenv("ANI_IMPORT_MINIO_SECRET")) == "" {
					return fmt.Errorf("ANI_IMPORT_MINIO_SECRET is required in Kubernetes import mode")
				}
			}
			importWorker = &worker.Worker{
				Store: workStore, Binder: postgres.NewImportBinder(pool, providers), Executor: executor,
				Finalizer: versionStore, TenantID: "", Owner: owner, Logger: logger,
			}
		}
	}
	var runners []server.WorkerRunner
	references, closeReferences, err := configuredInferenceReferences()
	if err != nil {
		return fmt.Errorf("configure Inference references: %w", err)
	}
	defer closeReferences()
	modelService.SetInferenceReferenceChecker(references)
	if importWorker != nil {
		modelService.SetImportNotifier(importWorker)
		runners = append(runners, importWorker)
	}
	app, err := buildAppWithModelService(&bc, logger, modelService, true, postgresReady, storageReady, runners...)
	if err != nil {
		return fmt.Errorf("build app: %w", err)
	}
	if err := app.Run(); err != nil {
		return fmt.Errorf("run app: %w", err)
	}
	return nil
}

func firstEnv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func newRuntimeLogger(writer io.Writer) *slog.Logger {
	handler := log.NewHandler(
		log.WithWriter(writer),
		log.WithFormat(log.FormatJSON),
		log.WithLevel(log.LevelInfo),
		log.WithAddSource(true),
		log.WithExtractor(tracing.TraceAttrs),
		log.WithFilter(log.FilterKey(
			"args",
			"authorization",
			"cookie",
			"credential",
			"password",
			"private_key",
			"set-cookie",
			"token",
		)),
	)
	return slog.New(handler).With(
		slog.String("service.id", id),
		slog.String("service.name", Name),
		slog.String("service.version", Version),
	)
}
