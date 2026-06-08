package gserv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"

	"go.oneofone.dev/oerrs"
	"go.oneofone.dev/otk"
)

// Common responses are pre-built response values for frequent use cases.
var (
	RespMethodNotAllowed Response = NewJSONErrorResponse(http.StatusMethodNotAllowed).Cached()
	RespNotFound         Response = NewJSONErrorResponse(http.StatusNotFound).Cached()
	RespForbidden        Response = NewJSONErrorResponse(http.StatusForbidden).Cached()
	RespBadRequest       Response = NewJSONErrorResponse(http.StatusBadRequest).Cached()
	RespOK               Response = NewJSONResponse("OK").Cached()
	RespEmpty            Response = CachedResponse(http.StatusNoContent, "", nil)
	RespPlainOK          Response = CachedResponse(http.StatusOK, "", nil)
	RespRedirectRoot     Response = Redirect("/", false)

	// Break can be returned from a handler to break the handler chain.
	// It does not write anything to the connection.
	Break Response = &cachedResp{code: -1}
)

// Response represents a generic return type for HTTP responses, with methods to determine the status code and write to the context.
type Response interface {
	Status() int
	WriteToCtx(ctx *Context) error
}

// PlainResponse returns a cached response with status 200 and the given content type and body.
func PlainResponse(contentType string, body any) Response {
	return CachedResponse(http.StatusOK, contentType, body)
}

// CachedResponse returns a cached response with the given HTTP status code, content type, and body.
func CachedResponse(code int, contentType string, body any) Response {
	if body == nil && code != http.StatusNoContent {
		body = http.StatusText(code)
	}

	var b []byte
	switch v := body.(type) {
	case nil:
	case []byte:
		b = v
	case string:
		b = otk.UnsafeBytes(v)
	case fmt.Stringer:
		b = otk.UnsafeBytes(v.String())
	case io.Reader:
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, v)
		b = buf.Bytes()
	default:
		v = otk.UnsafeBytes(fmt.Sprintf("%v", v))
	}

	return &cachedResp{
		ct:   contentType,
		body: b,
		code: code,
	}
}

// cachedResp is an internal cached response type.
type cachedResp struct {
	ct   string
	body []byte
	code int
}

func (r *cachedResp) Status() int { return r.code }
func (r *cachedResp) WriteToCtx(ctx *Context) error {
	if r.ct != "" {
		ctx.SetContentType(r.ct)
	}
	if r.code != 0 {
		ctx.WriteHeader(r.code)
	}
	_, err := ctx.Write(r.body)
	return err
}

func (r *cachedResp) MarshalJSON() ([]byte, error) {
	return r.body, nil
}

func (r *cachedResp) MarshalMsgPack() ([]byte, error) {
	return r.body, nil
}

func (r *cachedResp) Cached() Response { return r }

// ReadJSONResponse reads and decodes a JSON response from an io.ReadCloser, closing the body.
// dataValue is the target type for the response data field, for example:
//
//	r, err := ReadJSONResponse(res.Body, &map[string]*Stats{})
func ReadJSONResponse(rc io.ReadCloser, dataValue any) (r *JSONResponse, err error) {
	defer rc.Close()

	r = &JSONResponse{
		Data: dataValue,
	}

	if err = json.NewDecoder(rc).Decode(r); err != nil {
		return
	}

	if r.Success {
		return
	}

	var me MultiError
	for _, v := range r.Errors {
		me.Push(&v)
	}

	if err = me.Err(); err == nil {
		err = oerrs.String(http.StatusText(r.Code))
	}

	return
}

// JSONRequest makes an HTTP request and decodes the response as JSON into respData.
func JSONRequest(method, url string, reqData, respData any) (err error) {
	return otk.Request(method, "", url, reqData, func(r *http.Response) error {
		_, err := ReadJSONResponse(r.Body, respData)
		return err
	})
}

// Redirect returns a redirect response, using 302 if perm is false or 301 if perm is true.
func Redirect(url string, perm bool) Response {
	code := http.StatusFound
	if perm {
		code = http.StatusMovedPermanently
	}
	return RedirectWithCode(url, code)
}

// RedirectWithCode returns a redirect response with the specified HTTP status code.
func RedirectWithCode(url string, code int) Response {
	return redirResp{url, code}
}

// redirResp is an internal redirect response type.
type redirResp struct {
	url  string
	code int
}

func (r redirResp) Status() int { return r.code }
func (r redirResp) WriteToCtx(ctx *Context) error {
	if r.url == "" {
		return ErrInvalidURL
	}
	http.Redirect(ctx, ctx.Req, r.url, r.code)
	return nil
}

// File returns a response that serves the file at the given path with the specified content type.
func File(contentType, fp string) Response {
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(fp))
	}
	return fileResp{contentType, fp}
}

// fileResp is an internal file response type.
type fileResp struct {
	ct string
	fp string
}

func (f fileResp) Status() int { return 0 }
func (f fileResp) WriteToCtx(ctx *Context) error {
	if f.ct != "" {
		ctx.SetContentType(f.ct)
	}
	return ctx.File(f.fp)
}
