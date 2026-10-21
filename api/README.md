# Model Service API module

Module: `github.com/liangzai006/ani-model-service/api`

## Ownership

- `model/v1`: model/version management, import and trained artifact registration.
- `storage/v1`: object storage upload/download contracts.

`ModelReferenceService` belongs to Inference. Import its client from
`github.com/liangzai006/ani-inference-service/api/inference/v1`; do not copy that
protocol into this module. Service roots may consume each other's API modules;
the two API modules do not need to depend on each other for these RPCs.

The existing `model/v1/conf.proto` and `validate.go` also expose runtime
configuration types and validation. Their separation is a future migration,
not part of the reference-protocol ownership change.

## Consumption and releases

Consumers must pin an immutable published API version. The Model runtime
currently pins this downloadable version:

```bash
go get github.com/liangzai006/ani-model-service/api@v0.0.0-20261020102840-e6b26b580a9f
```

That historical version still contains the old `inference/v1` copy. The removal
in this working tree must be published before consumers can receive it. Migrate
any imports of `ani-model-service/api/inference/v1` to the Inference-owned
package first. Do not import both copies into the same executable: their
protobuf descriptors conflict during initialization.

Import Model clients using:

```go
import (
    modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
    storagev1 "github.com/liangzai006/ani-model-service/api/storage/v1"
)

modelClient := modelv1.NewModelServiceClient(conn)
storageClient := storagev1.NewStorageServiceClient(conn)
```

For a tagged release, create the tag at the repository root with the `api/`
prefix (for example, `api/v0.1.0`); consumers use the version without the prefix.
This is the tag convention, not a claim that `api/v0.1.0` is already published.
Package removal is a Go API compatibility change even when the gRPC wire
contract is unchanged. Document the migration when publishing the next version.
Never use a permanent local `replace` or workspace to stand in for a release.

## RPC examples

- Models: `CreateModel`, `GetModel`, `ListModels`, `DeleteModel`.
- Versions: `CreateModelVersion`, `GetModelVersion`, `ListModelVersions`,
  `DeleteModelVersion`.
- Trained artifacts: `PrepareTrainedArtifact`, `CompleteTrainedArtifact`,
  `AbortTrainedArtifact`.
- Storage: `CreateUploadURL`, `ObjectExists`, `VerifyChecksum`,
  `CreateDownloadURL`.

## Generation and verification

Run from the repository root:

```bash
make config                    # Regenerate protobuf output only
make generate                  # Also run Go generators and format Go source
make verify-api                # Tidy, tests, vet, build and module verification for api/
make verify-api-compatibility  # Load contracts together and test the service against local API
make verify                    # Generation, both modules and compatibility checks
```

The pinned workflow is `scripts/generate-api.sh`, using Buf v1.60.0,
protoc-gen-go v1.36.11 and protoc-gen-go-grpc v1.6.0. Do not edit generated Go
files by hand.

The root module's `go test ./...` does not traverse this nested module and uses
its published API dependency. `make test` explicitly tests both modules.
`make verify-api-compatibility` uses a disposable modfile pointing only the
Model API dependency at this checkout, loads every local API package together
with the published Inference contract to catch duplicate registration, and
runs the root tests against the local API. It does not modify repository
`go.mod`/`go.sum`, publish a version, or change normal build dependencies.
