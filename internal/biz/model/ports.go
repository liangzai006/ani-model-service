package model

import "context"

type Record struct {
	TenantID, ID, ExternalModelID, Name, DisplayName, Description, Source, Status string
	Capabilities                                                                  []byte
	TotalSizeBytes                                                                int64
	IdempotencyKey                                                                string
}

type Catalog interface {
	CreateModel(context.Context, string, string, string, string, string, string, []byte, string) (Record, error)
	GetModel(context.Context, string, string) (Record, error)
	ListModels(context.Context, string, string, int32) ([]Record, error)
	SoftDeleteModel(context.Context, string, string) error
}

type ExternalModelCatalog interface {
	GetModelByExternalID(context.Context, string, string) (Record, error)
}

type VersionReader interface {
	GetVersion(context.Context, string, string) (Version, error)
}

type VersionReferenceReader interface {
	GetVersionByExternalRef(context.Context, string, string, string) (Version, error)
}

type VersionStateStore interface {
	MarkReady(context.Context, string, string) error
	MarkError(context.Context, string, string, string) error
}

type VersionChecksumStore interface {
	SetChecksum(context.Context, string, string, string, int64) error
}

type VersionCatalog interface {
	VersionReader
	CreateVersion(context.Context, Version) (Version, error)
	ListVersions(context.Context, string, string, int32) ([]Version, error)
}

type ArtifactStore interface {
	CreateArtifact(context.Context, Artifact) (Artifact, error)
	GetArtifact(context.Context, string, string) (Artifact, error)
}
