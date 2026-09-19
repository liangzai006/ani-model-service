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
