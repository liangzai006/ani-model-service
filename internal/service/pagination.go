package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v3/errors"
	"github.com/google/uuid"
	modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
	"github.com/liangzai006/ani-model-service/internal/biz/model"
)

type listCursor struct {
	Version   int       `json:"v"`
	Scope     string    `json:"scope"`
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func cursorScope(parts ...string) string {
	b, _ := json.Marshal(parts)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// A cursor is a pagination position, never an identity or authorization token.
// Every store query still uses the tenant from the trusted principal.
func listPage(page *modelv1.CursorPageRequest, scope string) (model.ListOptions, error) {
	limit := page.GetLimit()
	if limit < 0 || limit > 1000 {
		return model.ListOptions{}, errors.BadRequest("INVALID_ARGUMENT", "page limit must be between 0 and 1000")
	}
	if limit == 0 {
		limit = 100
	}
	options := model.ListOptions{Limit: limit + 1}
	if page.GetCursor() == "" {
		return options, nil
	}
	invalid := errors.BadRequest("INVALID_CURSOR", "cursor is invalid or belongs to another list")
	if len(page.GetCursor()) > 2048 {
		return model.ListOptions{}, invalid
	}
	b, err := base64.RawURLEncoding.DecodeString(page.GetCursor())
	if err != nil {
		return model.ListOptions{}, invalid
	}
	var cursor listCursor
	if json.Unmarshal(b, &cursor) != nil || cursor.Version != 1 || cursor.Scope != scope || cursor.CreatedAt.IsZero() {
		return model.ListOptions{}, invalid
	}
	id, err := uuid.Parse(cursor.ID)
	if err != nil || id == uuid.Nil {
		return model.ListOptions{}, invalid
	}
	options.BeforeCreatedAt, options.BeforeID = cursor.CreatedAt, id.String()
	return options, nil
}

func nextCursor(scope string, createdAt time.Time, id string) string {
	b, _ := json.Marshal(listCursor{Version: 1, Scope: scope, CreatedAt: createdAt, ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}
