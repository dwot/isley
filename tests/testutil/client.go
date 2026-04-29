package testutil

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Client wraps an http.Client with a cookie jar and a base URL pointing
// at a TestServer. It is the testing equivalent of a browser session:
// cookies persist across requests, and the helper methods take request
// paths instead of full URLs so tests stay terse.
type Client struct {
	*http.Client

	t       *testing.T
	BaseURL string
}

// NewClient builds a Client bound to s with a fresh cookie jar. By
// default the client does NOT follow redirects so tests can assert on
// 302 destinations.
func (s *TestServer) NewClient(t *testing.T) *Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("NewClient: cookiejar: %v", err)
	}
	return &Client{
		Client: &http.Client{
			Jar: jar,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		t:       t,
		BaseURL: s.URL,
	}
}

// Get issues GET BaseURL+path and returns the response. Callers must
// close the body.
func (c *Client) Get(path string) *http.Response {
	c.t.Helper()
	resp, err := c.Client.Get(c.BaseURL + path)
	if err != nil {
		c.t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

// PostForm issues a POST with form-encoded body and returns the response.
// Callers must close the body.
func (c *Client) PostForm(path string, form url.Values) *http.Response {
	c.t.Helper()
	resp, err := c.Client.PostForm(c.BaseURL+path, form)
	if err != nil {
		c.t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

// APIGet issues GET BaseURL+path with the X-API-KEY header set. Caller
// closes the response body. Use the raw c.Do(APIReq(...)) form when a
// non-default content type or extra headers are needed.
func (c *Client) APIGet(t *testing.T, path, apiKey string) *http.Response {
	t.Helper()
	resp, err := c.Do(APIReq(t, http.MethodGet, c.BaseURL+path, apiKey, nil, ""))
	require.NoError(t, err)
	return resp
}

// APIPostJSON issues POST path with a JSON body and an X-API-KEY header.
// Caller closes the response body.
func (c *Client) APIPostJSON(t *testing.T, path, apiKey string, body interface{}) *http.Response {
	t.Helper()
	resp, err := c.Do(APIReq(t, http.MethodPost, c.BaseURL+path, apiKey,
		JSONBody(t, body), "application/json"))
	require.NoError(t, err)
	return resp
}

// APIDelete issues DELETE path with an X-API-KEY header. Caller closes
// the response body.
func (c *Client) APIDelete(t *testing.T, path, apiKey string) *http.Response {
	t.Helper()
	resp, err := c.Do(APIReq(t, http.MethodDelete, c.BaseURL+path, apiKey, nil, ""))
	require.NoError(t, err)
	return resp
}

// csrfTokenRE finds the hidden csrf_token input that login.html and the
// other auth-gated forms render. Test code uses it instead of parsing
// the full HTML, since the form is small and the markup is stable.
var csrfTokenRE = regexp.MustCompile(`name="csrf_token"\s+value="([^"]+)"`)

// metaCSRFTokenRE finds the <meta name="csrf-token" content="..."> tag
// emitted by common/header.html on dashboard pages. Frontend JS reads
// the same tag and forwards the value via the X-CSRF-Token header on
// XHR/fetch calls; tests for the session+CSRF round-trip do the same.
var metaCSRFTokenRE = regexp.MustCompile(`<meta\s+name="csrf-token"\s+content="([^"]+)"`)

// FetchCSRFToken issues GET path, reads the body, and returns the value
// of the first csrf_token input. Used by LoginAsAdmin to round-trip the
// session-bound CSRF token before submitting the login form.
func (c *Client) FetchCSRFToken(path string) string {
	c.t.Helper()
	resp := c.Get(path)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.t.Fatalf("FetchCSRFToken: GET %s: status %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("FetchCSRFToken: read body: %v", err)
	}
	m := csrfTokenRE.FindStringSubmatch(string(body))
	if len(m) < 2 {
		c.t.Fatalf("FetchCSRFToken: no csrf_token field found at %s; body starts: %s", path, head(body, 200))
	}
	return m[1]
}

// FetchMetaCSRFToken issues GET path on this client (which must already
// be logged in if path is auth-gated) and returns the token from the
// <meta name="csrf-token"> tag rendered by common/header.html. This is
// the token dashboard JS reads and submits via X-CSRF-Token, so tests
// using it exercise the session+CSRF contract real browser clients use.
//
// Use FetchCSRFToken for /login and /change-password — those forms POST
// as form-encoded and ship a hidden csrf_token input rather than the
// meta tag.
func (c *Client) FetchMetaCSRFToken(path string) string {
	c.t.Helper()
	resp := c.Get(path)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.t.Fatalf("FetchMetaCSRFToken: GET %s: status %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("FetchMetaCSRFToken: read body: %v", err)
	}
	m := metaCSRFTokenRE.FindStringSubmatch(string(body))
	if len(m) < 2 {
		c.t.Fatalf("FetchMetaCSRFToken: no <meta name=\"csrf-token\"> at %s; body starts: %s", path, head(body, 200))
	}
	return m[1]
}

// SessionPostJSON issues a JSON POST to path using the cookie jar's
// session cookie plus the supplied CSRF token in the X-CSRF-Token
// header — the contract the dashboard's frontend JS uses. Pass
// csrfToken="" to deliberately omit the header (the 403 path).
//
// Caller closes the response body. Companion to APIPostJSON, which
// authenticates via X-API-KEY and skips CSRF entirely.
func (c *Client) SessionPostJSON(t *testing.T, path, csrfToken string, body interface{}) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, JSONBody(t, body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}
	resp, err := c.Do(req)
	require.NoError(t, err)
	return resp
}

// LoginAndFetchCSRF logs in via LoginAsAdmin and then GETs csrfPagePath
// to extract the CSRF token rendered in <meta name="csrf-token">. The
// returned client carries the session cookie; callers issue mutating
// requests via SessionPostJSON (which forwards the token in
// X-CSRF-Token).
//
// This is the standard helper for session-path tests. Existing
// X-API-KEY tests are still correct — they exercise the API-key
// contract that operator scripts and external integrations use; the
// session path is what dashboard JS uses, and round-tripping it here
// keeps the cookie+CSRF contract from regressing silently.
//
// Callers must SeedAdmin (with the same password) before this runs.
func (s *TestServer) LoginAndFetchCSRF(t *testing.T, password, csrfPagePath string) (*Client, string) {
	t.Helper()
	c := s.LoginAsAdmin(t, password)
	return c, c.FetchMetaCSRFToken(csrfPagePath)
}

// LoginAsAdmin returns a Client that has authenticated as user "admin"
// with the given password. The session cookie is held in the client's
// cookie jar; subsequent requests on the returned client are
// authenticated. Fails the test if login does not redirect to "/".
//
// Callers should seed credentials first with SeedAdmin.
func (s *TestServer) LoginAsAdmin(t *testing.T, password string) *Client {
	t.Helper()
	c := s.NewClient(t)

	token := c.FetchCSRFToken("/login")

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", password)
	form.Set("csrf_token", token)

	resp := c.PostForm("/login", form)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("LoginAsAdmin: POST /login: got status %d, body: %s", resp.StatusCode, head(body, 400))
	}
	loc := resp.Header.Get("Location")
	if loc != "/" {
		t.Fatalf("LoginAsAdmin: POST /login: got redirect to %q, want %q", loc, "/")
	}
	return c
}

func head(body []byte, n int) string {
	if len(body) <= n {
		return string(body)
	}
	return strings.TrimSpace(string(body[:n])) + "..."
}
