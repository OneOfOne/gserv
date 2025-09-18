package gserv

import (
	"bytes"
	"log"
	"net/http"

	"go.oneofone.dev/oerrs"
)

func NewResponse[CodecT Codec](data any) *GenResponse[CodecT] {
	return &GenResponse[CodecT]{
		Code:    http.StatusOK,
		Success: true,
		Data:    data,
	}
}

// NewErrorResponse returns a new error response.
// each err can be:
// 1. string or []byte
// 2. error
// 3. Error / *Error
// 4. another response, its Errors will be appended to the returned Response.
// 5. MultiError
// 6. if errs is empty, it will call http.StatusText(code) and set that as the error.
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

// GenResponse is the default standard api response
type GenResponse[CodecT Codec] struct {
	Data    any     `json:"data,omitempty"`
	Errors  []Error `json:"errors,omitempty"`
	Code    int     `json:"code"`
	Success bool    `json:"success"`
}

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

// WriteToCtx writes the response to a ResponseWriter
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

func (r GenResponse[CodecT]) Cached() Response {
	var c CodecT
	var buf bytes.Buffer
	oerrs.Try(c.Encode(&buf, r))
	return &cachedResp{ct: c.ContentType(), code: r.Status(), body: buf.Bytes()}
}

// ErrorList returns an errors.ErrorList of this response's errors or nil.
// Deprecated: handled using MultiError
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
	PlainTextResponse = GenResponse[PlainTextCodec]
	JSONResponse      = GenResponse[JSONCodec]
	MsgpResponse      = GenResponse[MsgpCodec]

	CacheableResponse interface {
		Cached() Response
	}
)

// NewPlainResponse returns a new (json) success response (code 200) with the specific data
func NewPlainResponse(data any) *PlainTextResponse {
	return NewResponse[PlainTextCodec](data)
}

func NewPlainErrorResponse(code int, errs ...any) *PlainTextResponse {
	return NewErrorResponse[PlainTextCodec](code, errs...)
}

// NewJSONResponse returns a new (json) success response (code 200) with the specific data
func NewJSONResponse(data any) *JSONResponse {
	return NewResponse[JSONCodec](data)
}

func NewJSONErrorResponse(code int, errs ...any) *JSONResponse {
	return NewErrorResponse[JSONCodec](code, errs...)
}

// NewMsgpResponse returns a new (msgpack) success response (code 200) with the specific data
func NewMsgpResponse(data any) *MsgpResponse {
	return NewResponse[MsgpCodec](data)
}

func NewMsgpErrorResponse(code int, errs ...any) *MsgpResponse {
	return NewErrorResponse[MsgpCodec](code, errs...)
}
