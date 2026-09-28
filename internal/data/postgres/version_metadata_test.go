package postgres

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestVersionFromRowPreservesMetadata(t *testing.T) {
	now := time.Now().UTC()
	v := versionFromRow(ModelVersion{SizeBytes: 123, IsEncrypted: true, EncryptAlgo: "AES-256", EncryptHint: "key-a", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}}, "tenant-a")
	if v.SizeBytes != 123 || !v.IsEncrypted || v.EncryptAlgo != "AES-256" || v.EncryptHint != "key-a" || !v.CreatedAt.Equal(now) {
		t.Fatalf("version metadata lost: %+v", v)
	}
}

func TestResolvedVersionSizePrefersPositiveArtifactSize(t *testing.T) {
	if got := resolvedVersionSize(0, 279<<20); got != 279<<20 {
		t.Fatalf("resolved size = %d, want artifact size %d", got, 279<<20)
	}
	if got := resolvedVersionSize(123, 0); got != 123 {
		t.Fatalf("resolved size = %d, want version size %d when artifact size is invalid", got, 123)
	}
}
