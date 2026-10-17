package model

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidModel        = errors.New("invalid model")
	ErrInvalidModelVersion = errors.New("invalid model version")
	ErrInvalidBundleImport = errors.New("invalid model bundle import")
	ErrModelNotReady       = errors.New("model version is not ready")
)

// Version statuses are domain state values shared by import workers and
// persistence adapters. Keeping the terminal values here avoids adapters
// inventing a second vocabulary for aggregate state transitions.
const (
	VersionPending   = "pending"
	VersionImporting = "importing"
	VersionReady     = "ready"
	VersionError     = "error"
	VersionDeleted   = "deleted"
)

var checksumPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

type Version struct {
	TenantID, ID, ModelID, ExternalModelID, Version, Format, Status string
	ArtifactProvider, ArtifactReference, ArtifactSHA256             string
	EngineType, EngineImage, StartupCommand                         string
	StartupArgs                                                     []string
	IdempotencyKey                                                  string
	SizeBytes                                                       int64
	IsEncrypted                                                     bool
	EncryptAlgo, EncryptHint                                        string
	CreatedAt                                                       time.Time
}

// Artifact records the immutable object metadata associated with a model
// version. The object itself is owned by the external Storage service.
type Artifact struct {
	TenantID, ID, ModelVersionID string
	Provider, Reference, Format  string
	SHA256                       string
	SizeBytes                    int64
	IsEncrypted                  bool
	EncryptAlgo                  string
}

func (a Artifact) Validate() error {
	if a.TenantID == "" || a.ID == "" || a.ModelVersionID == "" || a.Provider == "" || a.Reference == "" || !checksumPattern.MatchString(a.SHA256) || a.SizeBytes < 0 {
		return ErrInvalidModelVersion
	}
	if a.Format != "safetensors" && a.Format != "gguf" && a.Format != "pytorch" {
		return ErrInvalidModelVersion
	}
	return nil
}

func ValidateModelName(name string) error {
	if len(name) == 0 || len(name) > 63 || strings.Trim(name, " ") != name {
		return ErrInvalidModel
	}
	for _, r := range name {
		if !(r == '-' || r == '.' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return ErrInvalidModel
		}
	}
	return nil
}

func (v Version) Validate() error {
	if v.TenantID == "" || v.ID == "" || v.ModelID == "" || v.Version == "" {
		return ErrInvalidModelVersion
	}
	if v.Format != "safetensors" && v.Format != "gguf" && v.Format != "pytorch" {
		return ErrInvalidModelVersion
	}
	return nil
}

func (v Version) DeploymentInput() error {
	if err := v.Validate(); err != nil {
		return err
	}
	if v.Status != VersionReady || v.ArtifactProvider == "" || v.ArtifactReference == "" || !checksumPattern.MatchString(v.ArtifactSHA256) {
		return ErrModelNotReady
	}
	return nil
}

func ValidateEngine(engine, command string, args []string) error {
	if engine == "" && command == "" && len(args) == 0 {
		return nil
	}
	if strings.TrimSpace(engine) == "" {
		return ErrInvalidModelVersion
	}
	if strings.TrimSpace(command) == "" || strings.ContainsAny(command, "\r\n") {
		return ErrInvalidModelVersion
	}
	return nil
}

func ValidateEngineDefaults(engine, image, command string) error {
	if strings.TrimSpace(engine) == "" || strings.TrimSpace(image) == "" || strings.TrimSpace(command) == "" || strings.ContainsAny(command, "\r\n") {
		return ErrInvalidModelVersion
	}
	return nil
}

// BundleImport is the complete model, version, and source input for one
// aggregate import. TenantID is supplied by the trusted caller context.
type BundleImport struct {
	TenantID, IdempotencyKey                        string
	Name, DisplayName, Description, ExternalModelID string
	Capabilities                                    []string
	Version, Format, EngineType, EngineImage        string
	StartupCommand                                  string
	Provider, RepoID, Revision                      string
}

func (b BundleImport) Validate() error {
	if b.TenantID == "" || b.IdempotencyKey == "" || b.RepoID == "" || ValidateModelName(b.Name) != nil {
		return ErrInvalidBundleImport
	}
	if b.Provider != "huggingface" && b.Provider != "modelscope" {
		return ErrInvalidBundleImport
	}
	if len(b.Capabilities) == 0 {
		return ErrInvalidBundleImport
	}
	for _, capability := range b.Capabilities {
		if strings.TrimSpace(capability) == "" {
			return ErrInvalidBundleImport
		}
	}
	if strings.TrimSpace(b.Version) == "" || strings.TrimSpace(b.Format) == "" || ValidateEngineDefaults(b.EngineType, b.EngineImage, b.StartupCommand) != nil {
		return ErrInvalidBundleImport
	}
	return nil
}

type BundleImportResult struct {
	Model                        Record
	Version                      Version
	TaskID, TaskType, TaskStatus string
}

type BundleImporter interface {
	ImportBundle(context.Context, BundleImport) (BundleImportResult, error)
}
