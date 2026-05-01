package core

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type CursorPage struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit"`
}

type CursorPageResult[T any] struct {
	Items      []T              `json:"items"`
	Pagination CursorPagination `json:"pagination"`
}

type CursorPagination struct {
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

func ParseCursorPage(r *http.Request) CursorPage {
	cursor := r.URL.Query().Get("cursor")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return CursorPage{Cursor: cursor, Limit: limit}
}

func EncodeCursor(createdAt, id string) string {
	return base64.URLEncoding.EncodeToString([]byte(createdAt + "|" + id))
}

func DecodeCursor(cursor string) (createdAt string, id string, err error) {
	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", fmt.Errorf("invalid cursor: %w", err)
	}
	const cursorParts = 2
	parts := strings.SplitN(string(data), "|", cursorParts)
	if len(parts) != cursorParts {
		return "", "", errors.New("invalid cursor format")
	}
	return parts[0], parts[1], nil
}

func NewCursorPageResult[T any](items []T, nextCursor string, hasMore bool) CursorPageResult[T] {
	if items == nil {
		items = []T{}
	}
	return CursorPageResult[T]{
		Items: items,
		Pagination: CursorPagination{
			NextCursor: nextCursor,
			HasMore:    hasMore,
		},
	}
}
