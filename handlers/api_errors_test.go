package handlers

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/logger"
	"isley/utils"
)

// loggerOnceForAPIErrors ensures logger.Log + utils.TranslationService
// are populated for tests in this file. Tests in package handlers can't
// import tests/testutil (cycle through app), so we mirror the harness
// init locally.
var loggerOnceForAPIErrors sync.Once

func ensureI18nForTests() {
	loggerOnceForAPIErrors.Do(func() {
		l := logrus.New()
		l.SetOutput(io.Discard)
		l.SetLevel(logrus.PanicLevel)
		logger.Log = l
		utils.Init("en")
	})
}

// runHandler runs a single Gin handler in a minimal Gin engine wired
// with the sessions middleware (which utils.GetLanguage requires) and
// an httptest.ResponseRecorder. Returns the recorder so callers can
// inspect status + body.
func runHandler(handler gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	store := cookie.NewStore([]byte("test-secret-32-byte-string-okok!"))
	r.Use(sessions.Sessions("isley_session", store))
	r.GET("/", handler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(rec, req)
	return rec
}

// decode parses the response body as a JSON object.
func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var body map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	return body
}

// ---------------------------------------------------------------------------
// T (translation lookup with key fallback)
// ---------------------------------------------------------------------------

func TestT_FallsBackToKeyWhenMissing(t *testing.T) {
	ensureI18nForTests()
	t.Parallel()

	// Run T inside a real Gin engine so sessions middleware is wired.
	var got string
	rec := runHandler(func(c *gin.Context) {
		got = T(c, "definitely-not-a-real-key")
		c.String(200, got)
	})
	assert.Equal(t, "definitely-not-a-real-key", got, "missing key should round-trip as-is")
	assert.Equal(t, 200, rec.Code)
}

// ---------------------------------------------------------------------------
// apiOK / error helpers — status code + JSON body shape
// ---------------------------------------------------------------------------

func TestAPIHelpers_StatusCodes(t *testing.T) {
	ensureI18nForTests()

	cases := []struct {
		name   string
		fn     func(*gin.Context, string)
		status int
		key    string // expected to be the JSON shape's value when no translation matches
		field  string // "message" for OK, "error" for the rest
	}{
		{"apiOK", apiOK, 200, "ok-key", "message"},
		{"apiBadRequest", apiBadRequest, 400, "bad-key", "error"},
		{"apiNotFound", apiNotFound, 404, "missing-key", "error"},
		{"apiForbidden", apiForbidden, 403, "forbidden-key", "error"},
		{"apiInternalError", apiInternalError, 500, "internal-key", "error"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := runHandler(func(c *gin.Context) { tc.fn(c, tc.key) })

			assert.Equal(t, tc.status, rec.Code)
			body := decode(t, rec)
			// With no translation table loaded for these keys, T() falls
			// back to the key itself. The JSON shape is the contract:
			// success → {"message": ...}; error → {"error": ...}.
			assert.Equal(t, tc.key, body[tc.field],
				"%s should put translated value under %q", tc.name, tc.field)
		})
	}
}
