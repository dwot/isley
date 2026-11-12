package apiv1_test

// Contract-test harness. Every /api/v1 response a test gets through
// contractClient is validated against openapi.yaml, and TestMain checks
// two kinds of coverage once all tests have run:
//
//   - every operation in the spec was exercised by at least one test, and
//   - every route mounted under /api/v1 is in the spec
//     (TestSpecCoversEveryRoute).
//
// A new endpoint therefore needs a spec entry and a contract test, or the
// suite fails.

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/stretchr/testify/require"

	"isley/apiv1"
	"isley/tests/testutil"
)

var (
	specOnce      sync.Once
	specValidator validator.Validator
	specOps       map[string]*regexp.Regexp // "GET /instance" -> path matcher
	specErr       error

	hitMu sync.Mutex
	hit   = map[string]bool{}
)

func loadSpec() {
	specOnce.Do(func() {
		doc, err := libopenapi.NewDocument(apiv1.Spec())
		if err != nil {
			specErr = err
			return
		}
		v, errs := validator.NewValidator(doc)
		if len(errs) > 0 {
			specErr = fmt.Errorf("build validator: %v", errs)
			return
		}
		model, err := doc.BuildV3Model()
		if err != nil {
			specErr = err
			return
		}
		specOps = map[string]*regexp.Regexp{}
		for path, item := range model.Model.Paths.PathItems.FromOldest() {
			pattern := "^" + regexp.MustCompile(`\\\{[^}]+\\\}`).ReplaceAllString(regexp.QuoteMeta(path), `[^/]+`) + "$"
			for method := range item.GetOperations().FromOldest() {
				specOps[strings.ToUpper(method)+" "+path] = regexp.MustCompile(pattern)
			}
		}
		specValidator = v
	})
}

func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 && specErr == nil && specOps != nil && os.Getenv("ISLEY_CONTRACT_SKIP_COVERAGE") == "" && !testing.Short() {
		var missed []string
		for op := range specOps {
			if !hit[op] {
				missed = append(missed, op)
			}
		}
		if len(missed) > 0 && runningFullSuite() {
			sort.Strings(missed)
			fmt.Fprintf(os.Stderr, "contract coverage: no test exercised these spec operations:\n  %s\n", strings.Join(missed, "\n  "))
			code = 1
		}
	}
	os.Exit(code)
}

// runningFullSuite is false when -run narrows the tests, so a focused run
// doesn't fail on coverage it never tried to reach.
func runningFullSuite() bool {
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.run") {
			return false
		}
	}
	return true
}

// contractClient sends requests to a test server and checks every
// response against the spec.
type contractClient struct {
	t      *testing.T
	server *testutil.TestServer
	client *testutil.Client
}

func newContractClient(t *testing.T, server *testutil.TestServer) *contractClient {
	t.Helper()
	loadSpec()
	require.NoError(t, specErr, "openapi.yaml must load")
	return &contractClient{t: t, server: server, client: server.NewClient(t)}
}

// response is a contract-checked response with its body read.
type response struct {
	*http.Response
	Body []byte
}

// do sends method path (relative to /api/v1) with optional headers and
// body, validates the response against the spec, and returns it.
func (cc *contractClient) do(method, path string, headers map[string]string, body []byte) response {
	t := cc.t
	t.Helper()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, cc.server.URL+apiv1.Prefix+path, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := cc.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	cc.validate(method, path, req, resp, raw)
	return response{Response: resp, Body: raw}
}

func (cc *contractClient) get(path, key string) response {
	cc.t.Helper()
	return cc.do(http.MethodGet, path, bearer(key), nil)
}

func bearer(key string) map[string]string {
	if key == "" {
		return nil
	}
	return map[string]string{"Authorization": "Bearer " + key}
}

// validate checks the response against the spec operation the path
// matches. A path no operation matches must be a JSON 404 envelope.
func (cc *contractClient) validate(method, path string, req *http.Request, resp *http.Response, body []byte) {
	t := cc.t
	t.Helper()

	opKey := ""
	for key, re := range specOps {
		if strings.HasPrefix(key, method+" ") && re.MatchString(strings.SplitN(path, "?", 2)[0]) {
			opKey = key
			break
		}
	}
	if opKey == "" {
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s %s is not in the spec and was not a 404", method, path)
		requireErrorEnvelope(t, body)
		return
	}
	hitMu.Lock()
	hit[opKey] = true
	hitMu.Unlock()

	// The validator matches request paths against the spec's paths, which
	// are relative to the server URL (/api/v1).
	vreq := req.Clone(req.Context())
	vreq.URL.Path = strings.TrimPrefix(req.URL.Path, apiv1.Prefix)
	vresp := *resp
	vresp.Body = io.NopCloser(bytes.NewReader(body))

	ok, errs := specValidator.ValidateHttpResponse(vreq, &vresp)
	if !ok {
		var msgs []string
		for _, e := range errs {
			msg := e.Message
			if e.Reason != "" {
				msg += ": " + e.Reason
			}
			for _, se := range e.SchemaValidationErrors {
				msg += fmt.Sprintf("\n      %s: %s", se.FieldPath, se.Reason)
			}
			msgs = append(msgs, msg)
		}
		t.Fatalf("%s %s -> %d does not match openapi.yaml:\n  %s\nbody: %s",
			method, path, resp.StatusCode, strings.Join(msgs, "\n  "), body)
	}
}
