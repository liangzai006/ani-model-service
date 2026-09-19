package postgres

import (
	"context"
	"errors"
	"testing"

	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
)

func TestDeletionWithoutReferencesFailsClosed(t *testing.T) {
	store := NewModelStore(nil)
	for _, delete := range []func(context.Context, string, string, modelbiz.ReferenceChecker) error{store.DeleteModel, store.DeleteVersion} {
		if err := delete(context.Background(), "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", nil); !errors.Is(err, modelbiz.ErrReferenceCheckUnavailable) {
			t.Fatalf("missing reference checker allowed deletion: %v", err)
		}
	}
}
