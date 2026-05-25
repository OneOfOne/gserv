package gserv

import (
	"fmt"
	"net/http"

	"go.oneofone.dev/otk"
)

// HTTPError is the interface for HTTP errors with a status code and message.
type HTTPError interface {
	Status() int
	Error() string
}

var (
	// ErrBadRequest indicates a bad request (400).
	ErrBadRequest = NewError(http.StatusBadRequest, "bad request")
	// ErrUnauthorized indicates an unauthorized request (401).
	ErrUnauthorized = NewError(http.StatusUnauthorized, "unauthorized")
	// ErrForbidden indicates a forbidden request (403).
	ErrForbidden = NewError(http.StatusForbidden, "the gates of time are closed")
	// ErrNotFound indicates a resource not found (404).
	ErrNotFound = NewError(http.StatusNotFound, "not found")
	// ErrTeaPot is a fun 418 error.
	ErrTeaPot = NewError(http.StatusTeapot, "I'm a teapot")

	// ErrInternal indicates an internal server error (500).
	ErrInternal = NewError(http.StatusInternalServerError, "internal error")
	// ErrNotImpl indicates a not implemented error (501).
	ErrNotImpl = NewError(http.StatusNotImplemented, "not implemented")
)

// Error is a standard HTTP error with an optional caller info.
type Error struct {
	Caller  *callerInfo `json:"caller,omitempty"`
	Message string      `json:"message,omitempty"`
	Code    int         `json:"code,omitempty"`
}

type callerInfo struct {
	Func string `json:"func,omitempty"`
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
}

// NewError creates a new HTTPError with the given status code and message.
func NewError(status int, msg any) HTTPError {
	e := Error{
		Code:    status,
		Message: fmt.Sprint(msg),
	}
	return e
}

// NewErrorWithCaller creates a new HTTPError with the given status code, message, and caller information.
func NewErrorWithCaller(status int, msg string, skip int) HTTPError {
	e := Error{
		Code:    status,
		Message: msg,
	}
	if skip > 0 {
		var c callerInfo
		c.Func, c.File, c.Line = otk.Caller(1+skip, true)
		e.Caller = &c
	}

	return e
}
func (e Error) Status() int   { return e.Code }
func (e Error) Error() string { return e.Message }
