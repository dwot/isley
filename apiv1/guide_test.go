package apiv1_test

import (
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuideIsServedWithoutAuth(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	resp, err := f.server.NewClient(t).Do(mustReq(t, http.MethodGet, f.server.URL+"/api/v1/docs"))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")
	assert.Contains(t, string(body), `href="/api/v1/openapi.yaml"`)
}

// Every endpoint the guide shows exists in the spec, so the guide can't
// drift from the API.
func TestGuideMatchesSpec(t *testing.T) {
	t.Parallel()
	loadSpec()
	require.NoError(t, specErr)
	html, err := os.ReadFile("guide.html")
	require.NoError(t, err)
	page := strings.ReplaceAll(string(html), "&amp;", "&")

	exists := func(method, path string) bool {
		for op, re := range specOps {
			m, _, _ := strings.Cut(op, " ")
			if (method == "" || m == method) && re.MatchString(path) {
				return true
			}
		}
		return false
	}

	// Calls in the examples: $ISLEY/api/v1/plants/12/activities?...
	calls := regexp.MustCompile(`\$ISLEY/api/v1(/[^\s"'?<]*)`).FindAllStringSubmatch(page, -1)
	require.NotEmpty(t, calls)
	for _, m := range calls {
		assert.True(t, exists("", m[1]), "the guide calls %s, which isn't in openapi.yaml", m[1])
	}
	// Endpoints named in the text: <code>POST /activities/bulk</code>.
	named := regexp.MustCompile(`<code>(GET|POST|PUT|PATCH|DELETE) (/[\w{}/-]+)`).FindAllStringSubmatch(page, -1)
	require.NotEmpty(t, named)
	for _, m := range named {
		assert.True(t, exists(m[1], m[2]), "the guide names %s %s, which isn't in openapi.yaml", m[1], m[2])
	}
}
