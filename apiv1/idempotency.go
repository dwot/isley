package apiv1

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/service"
)

// IdempotencyHeader is the request header that makes a POST safe to retry.
const IdempotencyHeader = "Idempotency-Key"

// capture records what a handler writes so it can be replayed.
type capture struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *capture) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *capture) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// idempotent makes a POST with an Idempotency-Key header run once per key:
// a retry with the same key and body gets the first (successful) response
// again, with Idempotent-Replayed: true. A failed request releases its
// key. Requests without the header are unaffected.
func (a *api) idempotent(c *gin.Context) {
	key := strings.TrimSpace(c.GetHeader(IdempotencyHeader))
	if c.Request.Method != http.MethodPost || key == "" {
		c.Next()
		return
	}
	if len(key) > 255 {
		invalid(c, map[string]string{IdempotencyHeader: "must be at most 255 characters"})
		return
	}
	// The whole body is hashed, so it is read here; handlers apply their
	// own, smaller limits. Nothing is ever truncated: a body over the
	// largest any endpoint accepts is refused outright.
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			abortWith(c, http.StatusRequestEntityTooLarge, CodePayloadTooLarge, "the request is larger than 51 MB", nil)
			return
		}
		invalid(c, map[string]string{"body": "could not be read"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	hash := requestHash(c.Request, body)

	ctx := c.Request.Context()
	apiKey := c.MustGet(contextKeyAuth).(*Key)
	stored, err := a.svc.Idempotency.Begin(ctx, apiKey.ID, key, hash)
	if err != nil {
		writeError(c, err)
		return
	}
	if stored != nil {
		c.Header("Idempotent-Replayed", "true")
		if stored.Location != "" {
			c.Header("Location", stored.Location)
		}
		c.Data(stored.Status, "application/json; charset=utf-8", stored.Body)
		c.Abort()
		return
	}

	w := &capture{ResponseWriter: c.Writer}
	c.Writer = w
	c.Next()

	// The client may have gone; the outcome still has to be recorded.
	bg := context.WithoutCancel(ctx)
	status := w.Status()
	if status >= 200 && status < 300 {
		err = a.svc.Idempotency.Finish(bg, apiKey.ID, key, storedFrom(status, w))
	} else {
		err = a.svc.Idempotency.Abandon(bg, apiKey.ID, key)
	}
	if err != nil {
		logger.Log.WithError(err).Error("Failed to record idempotent response")
	}
}

func storedFrom(status int, w *capture) service.StoredResponse {
	return service.StoredResponse{Status: status, Body: w.body.Bytes(), Location: w.Header().Get("Location")}
}

// requestHash identifies a request's content. A multipart form is hashed
// by its parts, not its bytes: the boundary is random, so a client that
// rebuilds the same form for a retry sends different bytes.
func requestHash(r *http.Request, body []byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s %s\n", r.Method, r.URL.Path)
	if !hashForm(h, r.Header.Get("Content-Type"), body) {
		h.Write(body)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// hashForm writes each part's name, filename and content digest to h, in
// order. It reports false, having written nothing, when body is not a
// readable multipart form.
func hashForm(h hash.Hash, contentType string, body []byte) bool {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		return false
	}
	form := sha256.New()
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return false
		}
		content := sha256.New()
		if _, err := io.Copy(content, p); err != nil {
			return false
		}
		// Length-prefixed, so one part can't run into the next.
		fmt.Fprintf(form, "%d:%s%d:%s", len(p.FormName()), p.FormName(), len(p.FileName()), p.FileName())
		form.Write(content.Sum(nil))
	}
	h.Write([]byte("multipart\n"))
	h.Write(form.Sum(nil))
	return true
}
