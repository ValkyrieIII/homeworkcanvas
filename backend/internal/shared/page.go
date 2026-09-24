package shared

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// PageCursor 记录已返回的最后一项，用于跨课程列表的稳定续页。
type PageCursor struct {
	Time *time.Time `json:"time"`
	ID   uuid.UUID  `json:"id"`
}

func DecodePageCursor(raw string) (PageCursor, error) {
	var cursor PageCursor
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err == nil {
		err = json.Unmarshal(data, &cursor)
	}
	if err != nil || cursor.ID == uuid.Nil {
		return PageCursor{}, JSONError("shared.invalid_cursor", "分页位置无效", 400)
	}
	return cursor, nil
}

func EncodePageCursor(cursor PageCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func PageLimit(raw string) (int, error) {
	if raw == "" {
		return 30, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 100 {
		return 0, JSONError("shared.invalid_limit", "每页数量须在 1 到 100 之间", 400)
	}
	return limit, nil
}
