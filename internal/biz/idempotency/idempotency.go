package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
)

var ErrInvalidKey = errors.New("invalid idempotency key")
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func ValidateKey(key string) error {
	if !keyPattern.MatchString(key) {
		return ErrInvalidKey
	}
	return nil
}
func Fingerprint(payload []byte) string         { h := sha256.Sum256(payload); return hex.EncodeToString(h[:]) }
func ReplayMatches(stored, current string) bool { return stored != "" && stored == current }
