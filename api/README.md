# Model Service Public gRPC Contract

Module: `github.com/liangzai006/ani-model-service/api`

This module contains the public gRPC API definitions for the ANI Model Service, including:

- **model/v1**: Model and ModelVersion management, trained artifact registration
- **storage/v1**: Object storage upload/download contracts
- **inference/v1**: Model reference types for inference workloads

## Usage

```go
import (
    modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
    storagev1 "github.com/liangzai006/ani-model-service/api/storage/v1"
)
```

## Structure

```
api/
├── go.mod              # Independent module for API contracts
├── model/v1/           # Model management APIs
│   ├── model.proto
│   ├── model.pb.go
│   └── model_grpc.pb.go
├── storage/v1/         # Storage APIs
│   ├── storage.proto
│   ├── storage.pb.go
│   └── storage_grpc.pb.go
└── inference/v1/       # Inference reference types
    ├── model_reference.proto
    └── model_reference.pb.go
```

## Key APIs

### Model Management

- `GetModel` / `ListModels` - Query model metadata
- `CreateModel` / `UpdateModel` - Manage model records
- `GetModelVersion` / `ListModelVersions` - Version management
- `PrepareTrainedArtifact` - Initialize artifact registration
- `CompleteTrainedArtifact` - Finalize registration with integrity verification
- `AbortTrainedArtifact` - Cancel in-progress registration

### Storage

- `PrepareUpload` - Generate signed upload URLs
- `PrepareDownload` - Generate signed download URLs
- Object verification with SHA256 checksums

## Generation

Generated files are committed. To regenerate from `.proto` sources:

```bash
make api
```

Generation uses:
- buf 1.72.0
- protoc-gen-go 1.36.12
- protoc-gen-go-grpc 1.6.2

Do not edit generated `*.pb.go` and `*_grpc.pb.go` files directly.

## Versioning

This module follows semantic versioning independently from the service implementation. Breaking changes to the gRPC contract require a major version bump or a new API version (e.g., `model/v2`).
