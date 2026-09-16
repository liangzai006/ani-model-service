package model

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrInvalidModel        = errors.New("invalid model")
	ErrInvalidModelVersion = errors.New("invalid model version")
	ErrModelNotReady       = errors.New("model version is not ready")
)

var checksumPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

type Version struct {
	TenantID, ID, ModelID, ExternalModelID, Version, Format, Status string
	ArtifactProvider, ArtifactReference, ArtifactSHA256             string
	EngineType, StartupCommand                                      string
	StartupArgs                                                     []string
	IdempotencyKey                                                  string
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
	if v.Status != "ready" || v.ArtifactProvider == "" || v.ArtifactReference == "" || !checksumPattern.MatchString(v.ArtifactSHA256) {
		return ErrModelNotReady
	}
	return nil
}

func ValidateEngine(engine, command string, args []string) error {
	if engine == "" && command == "" && len(args) == 0 {
		return nil
	}
	if engine != "vllm" && engine != "sglang" && engine != "tgi" {
		return ErrInvalidModelVersion
	}
	if strings.TrimSpace(command) == "" || strings.ContainsAny(command, "\r\n") {
		return ErrInvalidModelVersion
	}
	return nil
}
