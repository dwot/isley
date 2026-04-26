package testutil

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"testing"
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

// csrfTokenRE finds the hidden csrf_token input that login.html and the
// other auth-gated forms render. Test code uses it instead of parsing
// the full HTML, since the form is small and the markup is stable.
var csrfTokenRE = regexp.MustCompile(`name="csrf_token"\s+value="([^"]+)"`)

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
