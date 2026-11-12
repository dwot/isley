package apiv1

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/service"
)

// Error codes the API itself produces. Service errors carry their own
// codes (service.Error.Code).
const (
	CodeUnauthorized      = "unauthorized"
	CodeInsufficientScope = "insufficient_scope"
	CodeNotFound          = "not_found"
	CodeMethodNotAllowed  = "method_not_allowed"
	CodeInternal          = "internal_error"
	CodeUnavailable       = "unavailable"
	CodePayloadTooLarge   = "payload_too_large"
)

// errorBody is the envelope every non-2xx response uses.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// abortWith writes an error envelope and stops the handler chain.
func abortWith(c *gin.Context, status int, code, message string, details map[string]any) {
	c.AbortWithStatusJSON(status, errorBody{Error: errorDetail{Code: code, Message: message, Details: details}})
}

// writeError turns an error from a service into the right response. A
// *service.Error keeps its code; anything else is logged and reported as
// internal_error without its text, which may hold SQL or paths.
func writeError(c *gin.Context, err error) {
	if e, ok := service.AsError(err); ok {
		var details map[string]any
		if len(e.Fields) > 0 {
			details = map[string]any{"fields": e.Fields}
		}
		abortWith(c, statusFor(e.Kind), e.Code, e.Message, details)
		return
	}
	if errors.Is(err, context.Canceled) {
		// The client went away; nobody is listening for a body.
		c.Abort()
		return
	}
	logger.Log.WithError(err).WithField("path", c.FullPath()).Error("API v1 request failed")
	abortWith(c, http.StatusInternalServerError, CodeInternal, "internal error", nil)
}

func statusFor(k service.Kind) int {
	switch k {
	case service.KindNotFound:
		return http.StatusNotFound
	case service.KindInvalid:
		return http.StatusBadRequest
	case service.KindConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// apiTime is how every timestamp leaves the API: whole seconds, keeping
// the instance zone's offset.
func apiTime(t time.Time) time.Time { return t.Truncate(time.Second) }

// invalid reports request-level validation problems (bad query
// parameters) in the same shape as service validation errors.
func invalid(c *gin.Context, fields map[string]string) {
	writeError(c, service.Invalid(fields))
}
