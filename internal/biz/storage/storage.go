package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"
)

var (
	ErrChecksumMismatch = errors.New("storage checksum mismatch")
	ErrURLExpired       = errors.New("storage url expired")
	ErrProvider         = errors.New("storage provider error")
)

type UploadRequest struct {
	TenantID, ObjectRef, ContentType string
	SizeBytes                        int64
	ChecksumSHA256                   string
}

type DownloadRequest struct {
	TenantID, ObjectRef, Requester string
	TTL                            time.Duration
}

type SignedURL struct {
	URL       string
	ExpiresAt time.Time
}

// Port is the boundary to an external object storage service. Implementations
// must not create buckets or manage storage infrastructure lifecycle.
type Port interface {
	CreateUploadURL(context.Context, UploadRequest) (SignedURL, error)
	ObjectExists(context.Context, string, string) (bool, error)
	VerifyChecksum(context.Context, string, string, string) error
	CreateDownloadURL(context.Context, DownloadRequest) (SignedURL, error)
}

// BucketManager ensures the destination bucket for a tenant exists. It does
// not delete buckets or manage PVC/Storage service lifecycle.
type BucketManager interface {
	EnsureBucket(context.Context, string) error
}

// Uploader writes a stream through a signed URL issued by the external
// Storage service. It does not own buckets, filesystems, or object lifecycle.
type Uploader interface {
	Upload(context.Context, UploadRequest, io.Reader) error
}

func VerifyReader(r io.Reader, expected string) error {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return ErrChecksumMismatch
	}
	return nil
}

func ValidateSignedURL(u SignedURL, now time.Time) error {
	if u.URL == "" || !u.ExpiresAt.After(now) {
		return ErrURLExpired
	}
	return nil
}
