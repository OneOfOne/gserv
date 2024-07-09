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

// Common responses
var (
	RespMethodNotAllowed Response = NewJSONErrorResponse(http.StatusMethodNotAllowed).Cached()
	RespNotFound         Response = NewJSONErrorResponse(http.StatusNotFound).Cached()
	RespForbidden        Response = NewJSONErrorResponse(http.StatusForbidden).Cached()
	RespBadRequest       Response = NewJSONErrorResponse(http.StatusBadRequest).Cached()
	RespOK               Response = NewJSONResponse("OK").Cached()
	RespEmpty            Response = CachedResponse(http.StatusNoContent, "", nil)
	RespPlainOK          Response = CachedResponse(http.StatusOK, "", nil)
	RespRedirectRoot     Response = Redirect("/", false)

	// Break can be returned from a handler to break a handler chain.
	// It doesn't write anything to the connection.
	// if you reassign this, a wild animal will devour your face.
	Break Response = &cachedResp{code: -1}
)

// Response represents a generic return type for http responses.
type Response interface {
	Status() int
	WriteToCtx(ctx *Context) error
}

func PlainResponse(contentType string, body any) Response {
	return CachedResponse(http.StatusOK, contentType, body)
}

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

// ReadJSONResponse reads a response from an io.ReadCloser and closes the body.
// dataValue is the data type you're expecting, for example:
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

func JSONRequest(method, url string, reqData, respData any) (err error) {
	return otk.Request(method, "", url, reqData, func(r *http.Response) error {
		_, err := ReadJSONResponse(r.Body, respData)
		return err
	})
}

// Redirect returns a redirect Response.
// if perm is false it uses http.StatusFound (302), otherwise http.StatusMovedPermanently (302)
func Redirect(url string, perm bool) Response {
	code := http.StatusFound
	if perm {
		code = http.StatusMovedPermanently
	}
	return RedirectWithCode(url, code)
}

// RedirectWithCode returns a redirect Response with the specified status code.
func RedirectWithCode(url string, code int) Response {
	return redirResp{url, code}
}

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

// File returns a file response.
// example: return File("plain/html", "index.html")
func File(contentType, fp string) Response {
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(fp))
	}
	return fileResp{contentType, fp}
}

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
