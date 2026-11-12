package service

import (
	"errors"
	"fmt"
	"sort"
)

// Kind is the category of a service error. Adapters map it to their own
// status: HTTP code, MCP tool error, web toast.
type Kind int

const (
	// KindNotFound: the thing asked for does not exist.
	KindNotFound Kind = iota + 1
	// KindInvalid: the request was malformed or broke a rule; Fields says
	// which inputs.
	KindInvalid
	// KindConflict: the request is valid but clashes with current state
	// (a duplicate name, an in-use record).
	KindConflict
)

func (k Kind) String() string {
	switch k {
	case KindNotFound:
		return "not_found"
	case KindInvalid:
		return "invalid"
	case KindConflict:
		return "conflict"
	default:
		return fmt.Sprintf("kind(%d)", int(k))
	}
}

// Error is a failure the caller can act on.
//
// Code is stable and documented (it is part of the API contract), e.g.
// "plant_not_found". Message is English for logs and developers; adapters
// may show a translated message instead. Fields maps an input name to what
// is wrong with it, for KindInvalid.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Fields  map[string]string
}

func (e *Error) Error() string {
	if len(e.Fields) == 0 {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	names := make([]string, 0, len(e.Fields))
	for name := range e.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
	msg := fmt.Sprintf("%s: %s (", e.Code, e.Message)
	for i, name := range names {
		if i > 0 {
			msg += "; "
		}
		msg += name + ": " + e.Fields[name]
	}
	return msg + ")"
}

// NotFound returns a KindNotFound error, e.g.
// NotFound("plant_not_found", "plant 12 does not exist").
func NotFound(code, message string) *Error {
	return &Error{Kind: KindNotFound, Code: code, Message: message}
}

// Conflict returns a KindConflict error.
func Conflict(code, message string) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: message}
}

// Invalid returns a KindInvalid error with code "invalid_request". Build
// one with Validation when several fields can be wrong at once.
func Invalid(fields map[string]string) *Error {
	return &Error{Kind: KindInvalid, Code: CodeInvalidRequest, Message: "the request has invalid fields", Fields: fields}
}

// CodeInvalidRequest is the code of every KindInvalid error.
const CodeInvalidRequest = "invalid_request"

// Validation collects field problems so a caller sees all of them in one
// response.
//
//	var v service.Validation
//	if name == "" { v.Add("name", "is required") }
//	if err := v.Err(); err != nil { return err }
type Validation struct {
	fields map[string]string
}

// Add records a problem with field. The first problem per field wins.
func (v *Validation) Add(field, problem string) {
	if v.fields == nil {
		v.fields = map[string]string{}
	}
	if _, seen := v.fields[field]; !seen {
		v.fields[field] = problem
	}
}

// Err returns an Invalid error for the recorded problems, or nil if there
// are none.
func (v *Validation) Err() error {
	if len(v.fields) == 0 {
		return nil
	}
	return Invalid(v.fields)
}

// AsError returns err as a *Error if it is one (possibly wrapped).
func AsError(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// IsKind reports whether err is a *Error of kind k.
func IsKind(err error, k Kind) bool {
	e, ok := AsError(err)
	return ok && e.Kind == k
}
