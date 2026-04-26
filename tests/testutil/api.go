package testutil

// HTTP helpers used by handler-layer and integration tests.
//
// Each test file used to define its own apiReq / drainAndClose /
// jsonBody / buildMultipartBody triplet because Go forbids cross-package
// imports of `_test.go` symbols. The duplication had grown to ~20
// near-identical copies across handlers/*_http_test.go and
// tests/integration/*.go. Phase 2 of docs/TEST_PLAN.md consolidates
// them here so new tests can call testutil.* without copy-paste.

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// APIReq builds an http.Request with X-API-KEY set when apiKey is
// non-empty and Content-Type set when contentType is non-empty. body
// may be nil. The caller is responsible for sending the request via a
// Client and for closing the response body (DrainAndClose helps).
func APIReq(t *testing.T, method, url, apiKey string, body io.Reader, contentType string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	require.NoError(t, err)
	if apiKey != "" {
		req.Header.Set("X-API-KEY", apiKey)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req
}

// JSONBody marshals v into a *bytes.Buffer suitable as a JSON request
// body. Failure to encode the fixture is a test-author error and
// terminates the test.
func JSONBody(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(v))
	return &buf
}

// DrainAndClose discards and closes resp.Body. Safe to call with a nil
// response — useful when chained after a t.Cleanup or defer that may
// fire on the error path.
func DrainAndClose(resp *http.Response) {
	if resp == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// MultipartBody builds a multipart/form-data body containing one named
// file part. The returned contentType already includes the boundary
// header, so the caller can pass it straight into APIReq.
func MultipartBody(t *testing.T, fieldName, filename string, payload []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(fieldName, filename)
	require.NoError(t, err)
	_, err = part.Write(payload)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}
