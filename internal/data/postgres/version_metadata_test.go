package postgres

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
)

func TestVersionFromRowPreservesMetadata(t *testing.T) {
	now := time.Now().UTC()
	v := versionFromRow(ModelVersion{SizeBytes: 123, IsEncrypted: true, EncryptAlgo: "AES-256", EncryptHint: "key-a", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}}, "tenant-a")
	if v.SizeBytes != 123 || !v.IsEncrypted || v.EncryptAlgo != "AES-256" || v.EncryptHint != "key-a" || !v.CreatedAt.Equal(now) {
		t.Fatalf("version metadata lost: %+v", v)
	}
}

func TestVersionFingerprintIncludesEngineImage(t *testing.T) {
	v := modelbiz.Version{ModelID: "m", Version: "v1", Format: "gguf", EngineType: "vllm", EngineImage: "image-a", StartupCommand: "exec  vllm  serve '/models'"}
	other := v
	other.EngineImage = "image-b"
	if versionFingerprint(v) == versionFingerprint(other) {
		t.Fatal("engine image did not affect idempotency fingerprint")
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
