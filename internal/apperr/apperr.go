// Package apperr is the error type every user-visible error goes through.
//
// An Error carries a stable machine-readable Code (listed in ERRORS.md and
// registered in codes.go), optional Params that fill the `{name}` placeholders
// of the `error.<code>` text in the locales catalog, and the wrapped cause for
// the logs. The frontend receives only `{code, params}` (see MarshalJSON):
// the user-facing text always comes from the catalog, never from Err.
package apperr

import (
	"encoding/json"
	"errors"
)

// Code is a dotted lowercase `area.reason` identifier, never renamed once released.
type Code string

// Error is a user-visible error.
type Error struct {
	Code   Code
	Params map[string]string
	Err    error
}

// New returns an Error with the given code, wrapping err (which may be nil).
func New(code Code, err error) *Error {
	return &Error{Code: code, Err: err}
}

// With returns e with the param set, for chaining at the call site.
func (e *Error) With(name, value string) *Error {
	if e.Params == nil {
		e.Params = map[string]string{}
	}
	e.Params[name] = value
	return e
}

// Error is the English message for the logs: the code, then the cause.
func (e *Error) Error() string {
	if e.Err == nil {
		return string(e.Code)
	}
	return string(e.Code) + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// wire is what the frontend sees: no message, so no user text leaks from Go errors.
type wire struct {
	Code   Code              `json:"code"`
	Params map[string]string `json:"params"`
}

// MarshalJSON serializes the error as `{code, params}`; params is never null.
func (e *Error) MarshalJSON() ([]byte, error) {
	p := e.Params
	if p == nil {
		p = map[string]string{}
	}
	return json.Marshal(wire{Code: e.Code, Params: p})
}

// As returns err as an *Error, or an Internal error wrapping it when it has
// no code: anything that reaches the user without a code is a bug to fix.
func As(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return New(Internal, err)
}

// MarshalForFrontend is the Wails MarshalError hook: every error returned by a
// binding reaches the frontend as `{code, params}`.
func MarshalForFrontend(err error) []byte {
	b, jerr := json.Marshal(As(err))
	if jerr != nil {
		return nil
	}
	return b
}
