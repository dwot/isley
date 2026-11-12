package apiv1

import (
	"encoding/base64"
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// List is the envelope of every list response.
type List[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

// NewList wraps items; a nil slice becomes [] so the JSON is never null.
func NewList[T any](items []T, next *string) List[T] {
	if items == nil {
		items = []T{}
	}
	return List[T]{Items: items, NextCursor: next}
}

// Page is a parsed ?limit=&cursor= pair.
type Page struct {
	Limit int
	// Cursor is the decoded cursor, or nil on the first page.
	Cursor json.RawMessage
}

// parsePage reads ?limit= and ?cursor=. On bad input it writes a 400 and
// returns false.
func parsePage(c *gin.Context) (Page, bool) {
	p := Page{Limit: DefaultLimit}
	problems := map[string]string{}

	if s := c.Query("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > MaxLimit {
			problems["limit"] = "must be a whole number from 1 to " + strconv.Itoa(MaxLimit)
		} else {
			p.Limit = n
		}
	}
	if s := c.Query("cursor"); s != "" {
		raw, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil || !json.Valid(raw) {
			problems["cursor"] = "is not a cursor this API returned"
		} else {
			p.Cursor = raw
		}
	}
	if len(problems) > 0 {
		invalid(c, problems)
		return Page{}, false
	}
	return p, true
}

// encodeCursor turns a resource's position (any JSON-able value, e.g.
// {"after_id": 42}) into an opaque cursor string.
func encodeCursor(position any) (*string, error) {
	raw, err := json.Marshal(position)
	if err != nil {
		return nil, err
	}
	s := base64.RawURLEncoding.EncodeToString(raw)
	return &s, nil
}

// decodeCursor reads a cursor from Page into position. It returns false
// (and writes a 400) when the cursor doesn't fit the resource.
func decodeCursor(c *gin.Context, p Page, position any) bool {
	if p.Cursor == nil {
		return true
	}
	if err := json.Unmarshal(p.Cursor, position); err != nil {
		invalid(c, map[string]string{"cursor": "is not a cursor this API returned"})
		return false
	}
	return true
}
