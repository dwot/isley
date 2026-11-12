package apiv1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/logger"
	"isley/service"
)

func init() {
	gin.SetMode(gin.TestMode)
	if logger.Log == nil {
		l := logrus.New()
		l.SetOutput(io.Discard)
		logger.Log = l
	}
}

func testContext(target string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c, w
}

func decodeEnvelope(t *testing.T, w *httptest.ResponseRecorder) errorDetail {
	t.Helper()
	var body errorBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return body.Error
}

func TestWriteError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{service.NotFound("plant_not_found", "plant 3 does not exist"), 404, "plant_not_found"},
		{fmt.Errorf("wrapped: %w", service.Conflict("name_taken", "a zone with that name exists")), 409, "name_taken"},
		{service.Invalid(map[string]string{"name": "is required"}), 400, "invalid_request"},
		{errors.New("pq: relation \"secret_table\" does not exist"), 500, CodeInternal},
	}
	for _, tc := range cases {
		c, w := testContext("/api/v1/x")
		writeError(c, tc.err)
		assert.Equal(t, tc.status, w.Code, tc.code)
		got := decodeEnvelope(t, w)
		assert.Equal(t, tc.code, got.Code)
		assert.NotContains(t, w.Body.String(), "secret_table", "internal errors must not leak their text")
	}

	c, w := testContext("/api/v1/x")
	writeError(c, service.Invalid(map[string]string{"name": "is required"}))
	assert.Equal(t, map[string]any{"fields": map[string]any{"name": "is required"}}, decodeEnvelope(t, w).Details)

	// A cancelled request gets no body.
	c, w = testContext("/api/v1/x")
	writeError(c, fmt.Errorf("query: %w", context.Canceled))
	assert.True(t, c.IsAborted())
	assert.Empty(t, w.Body.String())
}

func TestParsePage(t *testing.T) {
	t.Parallel()
	c, _ := testContext("/api/v1/x")
	p, ok := parsePage(c)
	require.True(t, ok)
	assert.Equal(t, DefaultLimit, p.Limit)
	assert.Nil(t, p.Cursor)

	cursor, err := encodeCursor(map[string]int{"after_id": 42})
	require.NoError(t, err)
	c, _ = testContext("/api/v1/x?limit=200&cursor=" + *cursor)
	p, ok = parsePage(c)
	require.True(t, ok)
	assert.Equal(t, 200, p.Limit)
	var pos struct {
		AfterID int `json:"after_id"`
	}
	require.True(t, decodeCursor(c, p, &pos))
	assert.Equal(t, 42, pos.AfterID)

	for _, q := range []string{"limit=0", "limit=201", "limit=ten", "cursor=!!!", "cursor=bm90IGpzb24"} {
		c, w := testContext("/api/v1/x?" + q)
		_, ok := parsePage(c)
		assert.False(t, ok, q)
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
		assert.Equal(t, service.CodeInvalidRequest, decodeEnvelope(t, w).Code, q)
	}

	// A valid cursor of the wrong shape for the resource is a 400 too.
	other, err := encodeCursor([]int{1, 2})
	require.NoError(t, err)
	c, w := testContext("/api/v1/x?cursor=" + *other)
	p, ok = parsePage(c)
	require.True(t, ok)
	assert.False(t, decodeCursor(c, p, &pos))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestNewListNeverNull(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(NewList[int](nil, nil))
	require.NoError(t, err)
	assert.JSONEq(t, `{"items":[],"next_cursor":null}`, string(b))
}

func TestPresentedKey(t *testing.T) {
	t.Parallel()
	cases := map[string]map[string]string{
		"abc": {"Authorization": "Bearer abc"},
		"def": {"Authorization": "BEARER  def "},
		"ghi": {"X-API-KEY": " ghi "},
		"":    {"Authorization": "Token abc", "X-API-KEY": "ignored-when-authorization-is-set"},
	}
	for want, headers := range cases {
		c, _ := testContext("/api/v1/x")
		for k, v := range headers {
			c.Request.Header.Set(k, v)
		}
		assert.Equal(t, want, presentedKey(c), "%v", headers)
	}
}
