package handlers

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
	// cannadbDefaultBaseURL is the XRPC base for the public AppView API.
	// Overridable via the cannadb.base_url setting (trailing slash optional).
	cannadbDefaultBaseURL = "https://api.cannadb.net/xrpc/"

	// cannadbWebBaseURL is the human-facing site used to build "View on
	// CannaDB" links. Distinct host from the API.
	cannadbWebBaseURL = "https://cannadb.org"

	cannadbStrainCollection  = "org.cannadb.strain"
	cannadbBreederCollection = "org.cannadb.breeder"

	// cannadbMaxRetries bounds retries on 429/5xx before giving up.
	cannadbMaxRetries = 4
	// cannadbBackoffBase is the base delay for exponential backoff.
	cannadbBackoffBase = 500 * time.Millisecond
	// cannadbBackoffMax caps a single backoff sleep.
	cannadbBackoffMax = 8 * time.Second
	// cannadbResponseCap bounds an inbound response body. Mirrors the sensor
	// scan cap: timeouts bound wall-clock, this bounds memory.
	cannadbResponseCap = 4 * 1024 * 1024
)

// Version is the running Isley version, set from main at startup and used in
// the outbound User-Agent so CannaDB ops can identify Isley traffic. Defaults
// to "dev" for tests and unset builds.
var Version = "dev"

func cannadbUserAgent() string {
	return fmt.Sprintf("Isley/%s (+https://github.com/dwot/isley)", Version)
}

// cannadbBaseURL returns the configured base URL or the default, normalized to
// a single trailing slash.
func cannadbBaseURL(raw string) string {
	if strings.TrimSpace(raw) == "" {
		raw = cannadbDefaultBaseURL
	}
	return strings.TrimRight(raw, "/") + "/"
}

// ---------------------------------------------------------------------------
// Wire types
// ---------------------------------------------------------------------------

// cannadbError is the standard error envelope; dispatch on Code, not Message.
type cannadbError struct {
	Code              string `json:"error"`
	Message           string `json:"message"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
	HTTPStatus        int    `json:"-"`
}

func (e *cannadbError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("cannadb: %s (%s, http %d)", e.Message, e.Code, e.HTTPStatus)
	}
	return fmt.Sprintf("cannadb: %s (http %d)", e.Code, e.HTTPStatus)
}

// cannadbRecord is the full-record envelope returned by getStrain/getBreeder.
// Value is the verbatim publisher record JSON — parse leniently.
type cannadbRecord struct {
	URI       string          `json:"uri"`
	CID       string          `json:"cid"`
	DID       string          `json:"did"`
	IndexedAt string          `json:"indexedAt"`
	Value     json.RawMessage `json:"value"`
}

// cannadbRange is the {min,max} shape used by cycleTime/thc/etc.
type cannadbRange struct {
	Min *int `json:"min"`
	Max *int `json:"max"`
}

// cannadbStrainValue is the subset of the strain record Isley imports. Unknown
// fields are intentionally ignored (forward-compatible).
type cannadbStrainValue struct {
	Name             string        `json:"name"`
	Kind             string        `json:"kind"`
	ShortDescription string        `json:"shortDescription"`
	Description      string        `json:"description"`
	Breeder          string        `json:"breeder"` // at-uri, may be absent
	BreederName      string        `json:"breederName"`
	Autoflower       bool          `json:"autoflower"`
	IndicaSativa     *int          `json:"indicaSativa"` // 0=indica .. 100=sativa
	CycleTime        *cannadbRange `json:"cycleTime"`
	SourceURL        string        `json:"sourceUrl"`
	Parents          []string      `json:"parents"`     // at-uris
	ParentNames      []string      `json:"parentNames"` // display fallbacks
}

// cannadbBreederValue is the subset of the breeder record Isley imports.
type cannadbBreederValue struct {
	Name string `json:"name"`
}

// cannadbSearchResult is a summary row from searchStrains. description here is
// plain text (not markdown). similarity is the trigram score (0..1).
type cannadbSearchResult struct {
	URI         string  `json:"uri"`
	Name        string  `json:"name"`
	BreederName string  `json:"breederName"`
	Description string  `json:"description"`
	Similarity  float64 `json:"similarity"`
}

// cannadbSearchResponse wraps the searchStrains rows + pagination cursor.
// NOTE: the array key ("strains") and cursor are per the handoff's
// summary-view/pagination conventions; verify against the live API if results
// ever come back empty despite a 200.
type cannadbSearchResponse struct {
	Strains []cannadbSearchResult `json:"strains"`
	Cursor  string                `json:"cursor"`
}

// ---------------------------------------------------------------------------
// Web-link derivation
// ---------------------------------------------------------------------------

// CannadbWebURL is the exported wrapper used by the view layer to render a
// "View on CannaDB" link from a stored strain/breeder AT-URI.
func CannadbWebURL(atURI string) string { return cannadbWebURL(atURI) }

// cannadbWebURL turns a record AT-URI into its human-facing CannaDB page URL.
// at://<did>/<collection>/<rkey> -> https://cannadb.org/<kind>/<did>/<rkey>.
// Returns "" if the URI can't be parsed.
func cannadbWebURL(atURI string) string {
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
	if collection == cannadbBreederCollection {
		kind = "breeder"
	}
	return cannadbWebBaseURL + "/" + kind + "/" + did + "/" + rkey
}

// ---------------------------------------------------------------------------
// HTTP plumbing
// ---------------------------------------------------------------------------

// cannadbGet issues GET <base><method>?<params>, decoding a 200 body into out.
// It honors Retry-After and backs off with jitter on 429/5xx. url.Values
// encoding handles AT-URI escaping for the uri= param.
func cannadbGet(baseURL, method string, params url.Values, out interface{}) error {
	fieldLogger := logger.Log.WithField("func", "cannadbGet").WithField("method", method)

	endpoint := cannadbBaseURL(baseURL) + method
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= cannadbMaxRetries; attempt++ {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", cannadbUserAgent())
		req.Header.Set("Accept", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < cannadbMaxRetries {
				time.Sleep(cannadbBackoff(attempt))
				continue
			}
			return err
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, cannadbResponseCap))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt < cannadbMaxRetries {
				time.Sleep(cannadbBackoff(attempt))
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
			apiErr := decodeCannadbError(resp.StatusCode, body)
			lastErr = apiErr
			if attempt < cannadbMaxRetries {
				wait := cannadbRetryWait(attempt, resp.Header.Get("Retry-After"), apiErr.RetryAfterSeconds)
				fieldLogger.WithField("status", resp.StatusCode).WithField("wait", wait).
					Debug("CannaDB backpressure, backing off")
				time.Sleep(wait)
				continue
			}
			return apiErr

		default:
			// 4xx other than 429 — not retryable.
			return decodeCannadbError(resp.StatusCode, body)
		}
	}
	return lastErr
}

func decodeCannadbError(status int, body []byte) *cannadbError {
	apiErr := &cannadbError{HTTPStatus: status}
	_ = json.Unmarshal(body, apiErr) // best-effort; body may not be the envelope
	if apiErr.Code == "" {
		apiErr.Code = "Unknown"
	}
	return apiErr
}

// cannadbBackoff returns the exponential backoff delay for an attempt, with
// full jitter, capped at cannadbBackoffMax.
func cannadbBackoff(attempt int) time.Duration {
	d := cannadbBackoffBase << attempt
	if d > cannadbBackoffMax {
		d = cannadbBackoffMax
	}
	// Full jitter: random in [d/2, d].
	half := d / 2
	return half + time.Duration(rand.Int63n(int64(half)+1))
}

// cannadbRetryWait decides how long to wait before the next retry. A server's
// explicit Retry-After header wins and is honored exactly — including "0",
// which means "retry immediately" — so a cooperative server can keep retries
// tight. The envelope's retry_after_seconds is the next fallback, and absent
// both we use jittered exponential backoff.
func cannadbRetryWait(attempt int, header string, envelopeSeconds int) time.Duration {
	if header = strings.TrimSpace(header); header != "" {
		if secs, err := strconv.Atoi(header); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if envelopeSeconds > 0 {
		return time.Duration(envelopeSeconds) * time.Second
	}
	return cannadbBackoff(attempt)
}

// ---------------------------------------------------------------------------
// High-level calls
// ---------------------------------------------------------------------------

// cannadbSearchStrains runs a fuzzy name search, returning summary rows.
func cannadbSearchStrains(baseURL, query string, limit int) ([]cannadbSearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("limit", strconv.Itoa(limit))

	var out cannadbSearchResponse
	if err := cannadbGet(baseURL, "org.cannadb.searchStrains", params, &out); err != nil {
		return nil, err
	}
	return out.Strains, nil
}

// cannadbGetStrain fetches a full strain record by AT-URI.
func cannadbGetStrain(baseURL, uri string) (*cannadbRecord, *cannadbStrainValue, error) {
	params := url.Values{}
	params.Set("uri", uri)

	var rec cannadbRecord
	if err := cannadbGet(baseURL, "org.cannadb.getStrain", params, &rec); err != nil {
		return nil, nil, err
	}
	var val cannadbStrainValue
	if len(rec.Value) > 0 {
		if err := json.Unmarshal(rec.Value, &val); err != nil {
			return nil, nil, fmt.Errorf("cannadb: decode strain value: %w", err)
		}
	}
	return &rec, &val, nil
}

// cannadbGetBreeder fetches a full breeder record by AT-URI. Callers must
// tolerate a RecordNotFound error and fall back to breederName.
func cannadbGetBreeder(baseURL, uri string) (*cannadbRecord, *cannadbBreederValue, error) {
	params := url.Values{}
	params.Set("uri", uri)

	var rec cannadbRecord
	if err := cannadbGet(baseURL, "org.cannadb.getBreeder", params, &rec); err != nil {
		return nil, nil, err
	}
	var val cannadbBreederValue
	if len(rec.Value) > 0 {
		if err := json.Unmarshal(rec.Value, &val); err != nil {
			return nil, nil, fmt.Errorf("cannadb: decode breeder value: %w", err)
		}
	}
	return &rec, &val, nil
}
