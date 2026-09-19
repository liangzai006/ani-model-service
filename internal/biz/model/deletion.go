package model

import (
	"context"
	"errors"
)

var (
	ErrModelInUse                = errors.New("model has active inference references")
	ErrReferenceCheckUnavailable = errors.New("inference reference check unavailable")
)

// ReferenceChecker calls the versioned Inference contract using actual version IDs.
type ReferenceChecker interface {
	HasActiveVersionReferences(context.Context, string, []string) (bool, error)
}

// DeletionCatalog serializes deletion with model reads and version creation.
// A store must not delete when the external reference check is unknown.
type DeletionCatalog interface {
	DeleteModel(context.Context, string, string, ReferenceChecker) error
	DeleteVersion(context.Context, string, string, ReferenceChecker) error
}
