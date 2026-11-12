package apiv1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/gin-gonic/gin"

	"isley/service"
)

// maxBodyBytes caps JSON request bodies.
const maxBodyBytes = 1 << 20

// bindJSON decodes the request body into dst. Unknown fields are an error,
// so a typo in a field name is reported instead of silently ignored. On
// failure it writes a 400 and returns false.
func bindJSON(c *gin.Context, dst any) bool {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBodyBytes+1))
	if err != nil {
		invalid(c, map[string]string{"body": "could not be read"})
		return false
	}
	if len(body) > maxBodyBytes {
		invalid(c, map[string]string{"body": fmt.Sprintf("must be at most %d bytes", maxBodyBytes)})
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		invalid(c, map[string]string{"body": describeJSONError(err)})
		return false
	}
	if dec.More() {
		invalid(c, map[string]string{"body": "must be a single JSON object"})
		return false
	}
	return true
}

func describeJSONError(err error) string {
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(err, &typeErr) && typeErr.Field != "":
		return fmt.Sprintf("field %q must be %s", typeErr.Field, typeErr.Type.String())
	case errors.As(err, &syntaxErr):
		return "is not valid JSON"
	case errors.Is(err, io.EOF):
		return "is required"
	default:
		// "json: unknown field \"x\"" and friends.
		return err.Error()
	}
}

// pathID reads the {id} path parameter. A malformed id can't name
// anything, so it is a 404 like any other missing resource.
func pathID(c *gin.Context, notFoundCode, what string) (int, bool) {
	return pathParamID(c, "id", notFoundCode, what)
}

// pathParamID reads a positive integer path parameter, like pathID.
func pathParamID(c *gin.Context, param, notFoundCode, what string) (int, bool) {
	raw := c.Param(param)
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		writeError(c, service.NotFound(notFoundCode, fmt.Sprintf("%s %q does not exist", what, raw)))
		return 0, false
	}
	return id, true
}

// optional is a PATCH field: it records whether the client sent it at all,
// so "absent" (leave alone) and null (clear) can be told apart.
type optional[T any] struct {
	set   bool
	value *T
}

func (o *optional[T]) UnmarshalJSON(b []byte) error {
	o.set = true
	if string(b) == "null" {
		o.value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.value = &v
	return nil
}

func (o optional[T]) patch() service.Patch[T] {
	return service.Patch[T]{Set: o.set, Value: o.value}
}
