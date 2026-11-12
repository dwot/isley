// Package cannadb is a client for CannaDB (https://cannadb.org), the
// AT-Protocol strain and breeder database Isley imports strains from.
package cannadb

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"isley/logger"
)

// ---------------------------------------------------------------------------
// CannaDB public API client.
//
// CannaDB (https://cannadb.org) is an AT-Protocol-native strain/breeder
// database. Its AppView serves a public, unauthenticated, read-only HTTP API
// at api.cannadb.net. Isley uses it to import strain records into the local
// strain library. See planning/isley-api-integration-handoff.md for the full
// contract; this client implements only the subset Isley needs.
//
// No auth, no API key — every call is a plain GET.
// ---------------------------------------------------------------------------

const (
	// DefaultBaseURL is the XRPC base for the public AppView API.
	// Overridable via the cannadb.base_url setting (trailing slash optional).
	DefaultBaseURL = "https://api.cannadb.net/xrpc/"

	// webBaseURL is the human-facing site used to build "View on
	// CannaDB" links. Distinct host from the API.
	webBaseURL = "https://cannadb.org"

	strainCollection  = "org.cannadb.strain"
	breederCollection = "org.cannadb.breeder"

	// maxRetries bounds retries on 429/5xx before giving up.
	maxRetries = 4
	// backoffBase is the base delay for exponential backoff.
	backoffBase = 500 * time.Millisecond
	// backoffMax caps a single backoff sleep.
	backoffMax = 8 * time.Second
	// responseCap bounds an inbound response body. Mirrors the sensor
	// scan cap: timeouts bound wall-clock, this bounds memory.
	responseCap = 4 * 1024 * 1024
)

// httpClient bounds every call; retries add their own backoff on top.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// Version is the running Isley version, set from main at startup and used in
// the outbound User-Agent so CannaDB ops can identify Isley traffic. Defaults
// to "dev" for tests and unset builds.
var Version = "dev"

func userAgent() string {
	return fmt.Sprintf("Isley/%s (+https://github.com/dwot/isley)", Version)
}

// normalizeBaseURL returns the configured base URL or the default, normalized to
// a single trailing slash.
func normalizeBaseURL(raw string) string {
	if strings.TrimSpace(raw) == "" {
		raw = DefaultBaseURL
	}
	return strings.TrimRight(raw, "/") + "/"
}

// ---------------------------------------------------------------------------
// Wire types
// ---------------------------------------------------------------------------

// Error is the standard error envelope; dispatch on Code, not Message.
type Error struct {
	Code              string `json:"error"`
	Message           string `json:"message"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
	HTTPStatus        int    `json:"-"`
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("cannadb: %s (%s, http %d)", e.Message, e.Code, e.HTTPStatus)
	}
	return fmt.Sprintf("cannadb: %s (http %d)", e.Code, e.HTTPStatus)
}

// Record is the full-record envelope returned by getStrain/getBreeder.
// Value is the verbatim publisher record JSON — parse leniently.
type Record struct {
	URI       string          `json:"uri"`
	CID       string          `json:"cid"`
	DID       string          `json:"did"`
	IndexedAt string          `json:"indexedAt"`
	Value     json.RawMessage `json:"value"`
}

// Range is the {min,max} shape used by cycleTime/thc/etc.
type Range struct {
	Min *int `json:"min"`
	Max *int `json:"max"`
}

// StrainValue is the subset of the strain record Isley imports. Unknown
// fields are intentionally ignored (forward-compatible).
type StrainValue struct {
	Name             string   `json:"name"`
	Kind             string   `json:"kind"`
	ShortDescription string   `json:"shortDescription"`
	Description      string   `json:"description"`
	Breeder          string   `json:"breeder"` // at-uri, may be absent
	BreederName      string   `json:"breederName"`
	Autoflower       bool     `json:"autoflower"`
	IndicaSativa     *int     `json:"indicaSativa"` // 0=indica .. 100=sativa
	CycleTime        *Range   `json:"cycleTime"`
	SourceURL        string   `json:"sourceUrl"`
	Parents          []string `json:"parents"`     // at-uris
	ParentNames      []string `json:"parentNames"` // display fallbacks
}

// BreederValue is the subset of the breeder record Isley imports.
type BreederValue struct {
	Name string `json:"name"`
}

// SearchResult is a summary row from searchStrains. description here is
// plain text (not markdown). similarity is the trigram score (0..1).
type SearchResult struct {
	URI         string  `json:"uri"`
	Name        string  `json:"name"`
	BreederName string  `json:"breederName"`
	Description string  `json:"description"`
	Similarity  float64 `json:"similarity"`
}

// searchResponse wraps the searchStrains rows + pagination cursor.
// NOTE: the array key ("strains") and cursor are per the handoff's
// summary-view/pagination conventions; verify against the live API if results
// ever come back empty despite a 200.
type searchResponse struct {
	Strains []SearchResult `json:"strains"`
	Cursor  string         `json:"cursor"`
}

// ---------------------------------------------------------------------------
// Web-link derivation
// ---------------------------------------------------------------------------

// WebURL turns a record AT-URI into its human-facing CannaDB page URL.
// at://<did>/<collection>/<rkey> -> https://cannadb.org/<kind>/<did>/<rkey>.
// Returns "" if the URI can't be parsed.
func WebURL(atURI string) string {
	atURI = strings.TrimSpace(atURI)
	if !strings.HasPrefix(atURI, "at://") {
		return ""
	}
	s := strings.TrimPrefix(atURI, "at://")
	parts := strings.Split(s, "/")
	if len(parts) < 3 || parts[0] == "" {
		return ""
	}
	did := parts[0]
	collection := parts[1]
	rkey := parts[len(parts)-1]
	if rkey == "" {
		return ""
	}
	kind := "strain"
	if collection == breederCollection {
		kind = "breeder"
	}
	return webBaseURL + "/" + kind + "/" + did + "/" + rkey
}

// ---------------------------------------------------------------------------
// HTTP plumbing
// ---------------------------------------------------------------------------

// get issues GET <base><method>?<params>, decoding a 200 body into out.
// It honors Retry-After and backs off with jitter on 429/5xx. url.Values
// encoding handles AT-URI escaping for the uri= param.
func get(baseURL, method string, params url.Values, out interface{}) error {
	fieldLogger := logger.Log.WithField("func", "get").WithField("method", method)

	endpoint := normalizeBaseURL(baseURL) + method
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", userAgent())
		req.Header.Set("Accept", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				time.Sleep(backoff(attempt))
				continue
			}
			return err
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, responseCap))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt < maxRetries {
				time.Sleep(backoff(attempt))
				continue
			}
			return readErr
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("cannadb: decode %s: %w", method, err)
			}
			return nil

		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			apiErr := decodeError(resp.StatusCode, body)
			lastErr = apiErr
			if attempt < maxRetries {
				wait := retryWait(attempt, resp.Header.Get("Retry-After"), apiErr.RetryAfterSeconds)
				fieldLogger.WithField("status", resp.StatusCode).WithField("wait", wait).
					Debug("CannaDB backpressure, backing off")
				time.Sleep(wait)
				continue
			}
			return apiErr

		default:
			// 4xx other than 429 — not retryable.
			return decodeError(resp.StatusCode, body)
		}
	}
	return lastErr
}

func decodeError(status int, body []byte) *Error {
	apiErr := &Error{HTTPStatus: status}
	_ = json.Unmarshal(body, apiErr) // best-effort; body may not be the envelope
	if apiErr.Code == "" {
		apiErr.Code = "Unknown"
	}
	return apiErr
}

// backoff returns the exponential backoff delay for an attempt, with
// full jitter, capped at backoffMax.
func backoff(attempt int) time.Duration {
	d := backoffBase << attempt
	if d > backoffMax {
		d = backoffMax
	}
	// Full jitter: random in [d/2, d].
	half := d / 2
	return half + time.Duration(rand.Int63n(int64(half)+1))
}

// retryWait decides how long to wait before the next retry. A server's
// explicit Retry-After header wins and is honored exactly — including "0",
// which means "retry immediately" — so a cooperative server can keep retries
// tight. The envelope's retry_after_seconds is the next fallback, and absent
// both we use jittered exponential backoff.
func retryWait(attempt int, header string, envelopeSeconds int) time.Duration {
	if header = strings.TrimSpace(header); header != "" {
		if secs, err := strconv.Atoi(header); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if envelopeSeconds > 0 {
		return time.Duration(envelopeSeconds) * time.Second
	}
	return backoff(attempt)
}

// ---------------------------------------------------------------------------
// High-level calls
// ---------------------------------------------------------------------------

// SearchStrains runs a fuzzy name search, returning summary rows.
func SearchStrains(baseURL, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("limit", strconv.Itoa(limit))

	var out searchResponse
	if err := get(baseURL, "org.cannadb.searchStrains", params, &out); err != nil {
		return nil, err
	}
	return out.Strains, nil
}

// GetStrain fetches a full strain record by AT-URI.
func GetStrain(baseURL, uri string) (*Record, *StrainValue, error) {
	params := url.Values{}
	params.Set("uri", uri)

	var rec Record
	if err := get(baseURL, "org.cannadb.getStrain", params, &rec); err != nil {
		return nil, nil, err
	}
	var val StrainValue
	if len(rec.Value) > 0 {
		if err := json.Unmarshal(rec.Value, &val); err != nil {
			return nil, nil, fmt.Errorf("cannadb: decode strain value: %w", err)
		}
	}
	return &rec, &val, nil
}

// GetBreeder fetches a full breeder record by AT-URI. Callers must
// tolerate a RecordNotFound error and fall back to breederName.
func GetBreeder(baseURL, uri string) (*Record, *BreederValue, error) {
	params := url.Values{}
	params.Set("uri", uri)

	var rec Record
	if err := get(baseURL, "org.cannadb.getBreeder", params, &rec); err != nil {
		return nil, nil, err
	}
	var val BreederValue
	if len(rec.Value) > 0 {
		if err := json.Unmarshal(rec.Value, &val); err != nil {
			return nil, nil, fmt.Errorf("cannadb: decode breeder value: %w", err)
		}
	}
	return &rec, &val, nil
}
