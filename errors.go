package gserv

import (
	"fmt"
	"net/http"

	"go.oneofone.dev/otk"
)

type HTTPError interface {
	Status() int
	Error() string
}

var (
	ErrBadRequest   = NewError(http.StatusBadRequest, "bad request")
	ErrUnauthorized = NewError(http.StatusUnauthorized, "unauthorized")
	ErrForbidden    = NewError(http.StatusForbidden, "the gates of time are closed")
	ErrNotFound     = NewError(http.StatusNotFound, "not found")
	ErrTeaPot       = NewError(http.StatusTeapot, "I'm a teapot")

	ErrInternal = NewError(http.StatusInternalServerError, "internal error")
	ErrNotImpl  = NewError(http.StatusNotImplemented, "not implemented")
)

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

func NewError(status int, msg any) HTTPError {
	e := Error{
		Code:    status,
		Message: fmt.Sprint(msg),
	}
	return e
}

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
