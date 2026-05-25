package gserv

import (
	"bytes"
	"log"
	"net/http"

	"go.oneofone.dev/oerrs"
)

// NewResponse creates a new successful response with status code 200 and the given data.
func NewResponse[CodecT Codec](data any) *GenResponse[CodecT] {
	return &GenResponse[CodecT]{
		Code:    http.StatusOK,
		Success: true,
		Data:    data,
	}
}

// NewErrorResponse creates a new error response with the given status code.
// Each err argument can be:
// 1. string or []byte — used as the error message.
// 2. error — its Error() method is used.
// 3. Error or *Error — appended directly.
// 4. another Response — its Errors are appended to this response.
// 5. MultiError — each error is recursively appended.
// If errs is empty, http.StatusText(code) is used as the error message.
func NewErrorResponse[CodecT Codec](code int, errs ...any) (r *GenResponse[CodecT]) {
	if len(errs) == 0 {
		errs = append(errs, http.StatusText(code))
	}

	r = &GenResponse[CodecT]{
		Code:   code,
		Errors: make([]Error, 0, len(errs)),
	}

	for _, err := range errs {
		r.appendErr(err)
	}

	return r
}

// GenResponse is the default standard API response type, generic over the codec used for encoding.
type GenResponse[CodecT Codec] struct {
	Data    any     `json:"data,omitempty"`
	Errors  []Error `json:"errors,omitempty"`
	Code    int     `json:"code"`
	Success bool    `json:"success"`
}

// Status returns the HTTP status code for this response. If Code is 0, it defaults to BadRequest when there are errors, or OK otherwise.
func (r GenResponse[CodecT]) Status() int {
	if r.Code == 0 {
		if len(r.Errors) > 0 {
			return http.StatusBadRequest
		} else {
			return http.StatusOK
		}
	}
	return r.Code
}

// WriteToCtx writes the response's headers and body to the given Context.
func (r GenResponse[CodecT]) WriteToCtx(ctx *Context) error {
	switch r.Code {
	case 0:
		if len(r.Errors) > 0 {
			r.Code = http.StatusBadRequest
		} else {
			r.Code = http.StatusOK
		}

	case http.StatusNoContent: // special case
		ctx.WriteHeader(http.StatusNoContent)
		return nil
	}

	r.Success = r.Code >= http.StatusOK && r.Code < http.StatusBadRequest

	var c CodecT
	ctx.SetContentType(c.ContentType())
	ctx.WriteHeader(r.Code)

	return c.Encode(ctx, &r)
}

// Cached returns a cached version of this response for use with the CacheableResponse interface.
func (r GenResponse[CodecT]) Cached() Response {
	var c CodecT
	var buf bytes.Buffer
	oerrs.Try(c.Encode(&buf, r))
	return &cachedResp{ct: c.ContentType(), code: r.Status(), body: buf.Bytes()}
}

// ErrorList returns an errors.ErrorList of this response's errors or nil.
// Deprecated: use MultiError instead.
func (r *GenResponse[CodecT]) ErrorList() *oerrs.ErrorList {
	if len(r.Errors) == 0 {
		return nil
	}
	var el oerrs.ErrorList
	for _, err := range r.Errors {
		el.PushIf(&err)
	}
	return &el
}

func (r *GenResponse[CodecT]) appendErr(err any) {
	switch v := err.(type) {
	case Error:
		r.Errors = append(r.Errors, v)
	case *Error:
		r.Errors = append(r.Errors, *v)
	case string:
		r.Errors = append(r.Errors, Error{Message: v})
	case []byte:
		r.Errors = append(r.Errors, Error{Message: string(v)})
	case *JSONResponse:
		r.Errors = append(r.Errors, v.Errors...)
	case MultiError:
		for _, err := range v {
			r.appendErr(err)
		}
	case error:
		r.Errors = append(r.Errors, Error{Message: v.Error()})
	default:
		log.Panicf("unsupported error type (%T): %v", v, v)
	}
}

type (
	// PlainTextResponse is a GenResponse using the PlainTextCodec.
	PlainTextResponse = GenResponse[PlainTextCodec]

	// JSONResponse is a GenResponse using the JSONCodec.
	JSONResponse = GenResponse[JSONCodec]

	// MsgpResponse is a GenResponse using the MsgpCodec.
	MsgpResponse = GenResponse[MsgpCodec]

	// CacheableResponse is an interface for responses that can be cached.
	CacheableResponse interface {
		Cached() Response
	}
)

// NewPlainResponse creates a new successful (code 200) plain text response with the given data.
func NewPlainResponse(data any) *PlainTextResponse {
	return NewResponse[PlainTextCodec](data)
}

// NewPlainErrorResponse creates a new error plain text response with the given status code and errors.
func NewPlainErrorResponse(code int, errs ...any) *PlainTextResponse {
	return NewErrorResponse[PlainTextCodec](code, errs...)
}

// NewJSONResponse creates a new successful (code 200) JSON response with the given data.
func NewJSONResponse(data any) *JSONResponse {
	return NewResponse[JSONCodec](data)
}

// NewJSONErrorResponse creates a new error JSON response with the given status code and errors.
func NewJSONErrorResponse(code int, errs ...any) *JSONResponse {
	return NewErrorResponse[JSONCodec](code, errs...)
}

// NewMsgpResponse creates a new successful (code 200) msgpack response with the given data.
func NewMsgpResponse(data any) *MsgpResponse {
	return NewResponse[MsgpCodec](data)
}

// NewMsgpErrorResponse creates a new error msgpack response with the given status code and errors.
func NewMsgpErrorResponse(code int, errs ...any) *MsgpResponse {
	return NewErrorResponse[MsgpCodec](code, errs...)
}
