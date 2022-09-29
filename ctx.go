package gserv

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.oneofone.dev/genh"
	"go.oneofone.dev/gserv/internal"
	"go.oneofone.dev/gserv/router"
	"go.oneofone.dev/oerrs"
	"go.oneofone.dev/otk"
)

var (
	_ http.ResponseWriter = (*Context)(nil)
	_ http.Flusher        = (*Context)(nil)
	_ io.StringWriter     = (*Context)(nil)
)

const (
	// ErrDir is Returned from ctx.File when the path is a directory not a file.
	ErrDir = oerrs.String("file is a directory")

	// ErrInvalidURL gets returned on invalid redirect urls.
	ErrInvalidURL = oerrs.String("invalid redirect error")

	// ErrEmptyCallback is returned when a callback is empty
	ErrEmptyCallback = oerrs.String("empty callback")

	// ErrEmptyData is returned when the data payload is empty
	ErrEmptyData = oerrs.String("empty data")
)

// Context is the default context passed to handlers
// it is not thread safe and should never be used outside the handler
type Context struct {
	http.ResponseWriter
	Req          *http.Request
	bytesWritten int

	s     *Server
	Codec Codec

	data   M
	nextMW func()
	next   func()

	ReqQuery url.Values
	Params   router.Params

	status             int
	hijackServeContent bool
	done               bool
}

// Param is a shorthand for ctx.Params.Get(name).
func (ctx *Context) Param(key string) string {
	return ctx.Params.Get(key)
}

// Query is a shorthand for ctx.Req.URL.Query().Get(key).
func (ctx *Context) Query(key string) string {
	return ctx.ReqQuery.Get(key)
}

// QueryDefault returns the query key or a default value.
func (ctx *Context) QueryDefault(key, def string) string {
	if v := ctx.ReqQuery.Get(key); v != "" {
		return v
	}
	return def
}

// Get returns a context value
func (ctx *Context) Get(key string) any {
	return ctx.data[key]
}

// Set sets a context value, useful in passing data to other handlers down the chain
func (ctx *Context) Set(key string, val any) {
	ctx.data[key] = val
}

// WriteReader outputs the data from the passed reader with optional content-type.
func (ctx *Context) WriteReader(contentType string, r io.Reader) (int64, error) {
	if contentType != "" {
		ctx.SetContentType(contentType)
	}

	return io.Copy(ctx, r)
}

// File serves a file using http.ServeContent.
// See http.ServeContent.
func (ctx *Context) File(fp string) error {
	ctx.hijackServeContent = true
	http.ServeFile(ctx, ctx.Req, fp)

	return nil
}

// Path is a shorthand for ctx.Req.URL.EscapedPath().
func (ctx *Context) Path() string {
	return ctx.Req.URL.EscapedPath()
}

// SetContentType sets the responses's content-type.
func (ctx *Context) SetContentType(typ string) {
	if typ == "" {
		return
	}
	h := ctx.Header()
	h.Set(contentTypeHeader, typ)
}

// ReqHeader returns the request header.
func (ctx *Context) ReqHeader(key string) string {
	return ctx.Req.Header.Get(key)
}

// ContentType returns the request's content-type.
func (ctx *Context) ContentType() string {
	return ctx.ReqHeader(contentTypeHeader)
}

// Read is a QoL shorthand for ctx.Req.Body.Read.
// Context implements io.Reader
func (ctx *Context) Read(p []byte) (int, error) {
	return ctx.Req.Body.Read(p)
}

// CloseBody closes the request body.
func (ctx *Context) CloseBody() error {
	return ctx.Req.Body.Close()
}

// BindJSON parses the request's body as json, and closes the body.
// Note that unlike gin.Context.Bind, this does NOT verify the fields using special tags.
func (ctx *Context) BindJSON(out any) error {
	return ctx.BindCodec(JSONCodec{}, out)
}

// BindMsgpoack parses the request's body as msgpack, and closes the body.
// Note that unlike gin.Context.Bind, this does NOT verify the fields using special tags.
func (ctx *Context) BindMsgpack(out any) error {
	return ctx.BindCodec(MsgpCodec{}, out)
}

// BindCodec parses the request's body as msgpack, and closes the body.
// Note that unlike gin.Context.BindCodec, this does NOT verify the fields using special tags.
func (ctx *Context) BindCodec(c Codec, out any) error {
	c = genh.FirstNonZero(c, ctx.Codec, DefaultCodec)
	err := c.Decode(ctx, out)
	ctx.CloseBody()
	return err
}

// Bind parses the request's body as msgpack, and closes the body.
// Note that unlike gin.Context.Bind, this does NOT verify the fields using special tags.
func (ctx *Context) Bind(out any) error {
	var c Codec
	ct := ctx.ContentType()
	switch {
	case strings.Contains(ct, "json"):
		c = JSONCodec{}
	case strings.Contains(ct, "msgpack"):
		c = MsgpCodec{}
	default:
		c = genh.FirstNonZero(ctx.Codec, DefaultCodec)
	}

	err := c.Decode(ctx, out)
	ctx.CloseBody()
	if err != nil {
		err = oerrs.Errorf("error decoding (%s): %w", ct, err)
	}
	return err
}

// Printf is a QoL function to handle outputing plain strings with optional fmt.Printf-style formating.
// calling this function marks the Context as done, meaning any returned responses won't be written out.
func (ctx *Context) Printf(code int, contentType, s string, args ...any) (int, error) {
	ctx.done = true

	if contentType == "" {
		contentType = MimePlain
	}

	ctx.SetContentType(contentType)

	if code > 0 {
		ctx.WriteHeader(code)
	}

	return fmt.Fprintf(ctx, s, args...)
}

// JSON outputs a json object, it is highly recommended to return *Response rather than use this directly.
// calling this function marks the Context as done, meaning any returned responses won't be written out.
func (ctx *Context) JSON(code int, indent bool, v any) error {
	return ctx.EncodeCodec(JSONCodec{indent}, code, v)
}

// Msgpack outputs a msgp object, it is highly recommended to return *Response rather than use this directly.
// calling this function marks the Context as done, meaning any returned responses won't be written out.
func (ctx *Context) Msgpack(code int, v any) error {
	return ctx.EncodeCodec(MsgpCodec{}, code, v)
}

func (ctx *Context) EncodeCodec(c Codec, code int, v any) error {
	c = genh.FirstNonZero(c, ctx.Codec, DefaultCodec)
	ctx.done = true
	ctx.SetContentType(c.ContentType())

	if code > 0 {
		ctx.WriteHeader(code)
	}
	return c.Encode(ctx, v)
}

func (ctx *Context) Encode(code int, v any) error {
	var c Codec
	ct := ctx.ContentType()
	switch {
	case strings.Contains(ct, "json"):
		c = JSONCodec{}
	case strings.Contains(ct, "msgpack"):
		c = MsgpCodec{}
	default:
		c = genh.FirstNonZero(ctx.Codec, DefaultCodec)
	}

	ctx.done = true
	ctx.SetContentType(c.ContentType())

	if code > 0 {
		ctx.WriteHeader(code)
	}
	return c.Encode(ctx, v)
}

// ClientIP returns the current client ip, accounting for X-Real-Ip and X-forwarded-For headers as well.
func (ctx *Context) ClientIP() string {
	h := ctx.Req.Header

	// handle proxies
	if ip := h.Get("X-Real-Ip"); ip != "" {
		return strings.TrimSpace(ip)
	}

	if ip := h.Get("X-Forwarded-For"); ip != "" {
		if index := strings.IndexByte(ip, ','); index >= 0 {
			if ip = strings.TrimSpace(ip[:index]); len(ip) > 0 {
				return ip
			}
		}

		if ip = strings.TrimSpace(ip); ip != "" {
			return ip
		}
	}

	if ip, _, err := net.SplitHostPort(strings.TrimSpace(ctx.Req.RemoteAddr)); err == nil {
		return ip
	}

	return ""
}

// NextMiddleware is a middleware-only func to execute all the other middlewares in the group and return before the handlers.
// will panic if called from a handler.
func (ctx *Context) NextMiddleware() {
	if ctx.nextMW != nil {
		ctx.nextMW()
	}
}

// NextHandler is a func to execute all the handlers in the group up until one returns a Response.
func (ctx *Context) NextHandler() {
	if ctx.next != nil {
		ctx.next()
	}
}

// Next is a QoL function that calls NextMiddleware() then NextHandler() if NextMiddleware() didn't return a response.
func (ctx *Context) Next() {
	ctx.NextMiddleware()
	ctx.NextHandler()
}

// WriteHeader and Write are to implement ResponseWriter and allows ghetto hijacking of http.ServeContent errors,
// without them we'd end up with plain text errors, we wouldn't want that, would we?
// WriteHeader implements http.ResponseWriter
func (ctx *Context) WriteHeader(s int) {
	if ctx.status = s; ctx.hijackServeContent && ctx.status >= http.StatusBadRequest {
		return
	}
	
	ctx.ResponseWriter.WriteHeader(s)
}

// Write implements http.ResponseWriter
func (ctx *Context) Write(p []byte) (int, error) {
	if ctx.hijackServeContent && ctx.status >= http.StatusBadRequest {
		ctx.hijackServeContent = false
		NewJSONErrorResponse(ctx.status, p).WriteToCtx(ctx)
		return len(p), nil
	}

	ctx.done = true
	ctx.bytesWritten += len(p)
	return ctx.ResponseWriter.Write(p)
}

// BytesWritten is the amount of bytes written from the body.
func (ctx *Context) BytesWritten() int {
	return ctx.bytesWritten
}

// Write implements io.StringWriter
func (ctx *Context) WriteString(p string) (int, error) {
	return ctx.ResponseWriter.Write(otk.UnsafeBytes(p))
}

// Write implements http.Flusher
func (ctx *Context) Flush() {
	ctx.ResponseWriter.(http.Flusher).Flush()
}

// Status returns last value written using WriteHeader.
func (ctx *Context) Status() int {
	if ctx.status == 0 {
		ctx.status = http.StatusOK
	}

	return ctx.status
}

// MultipartReader is like Request.MultipartReader but supports multipart/*, not just form-data
func (ctx *Context) MultipartReader() (*multipart.Reader, error) {
	req := ctx.Req

	v := req.Header.Get(contentTypeHeader)
	if v == "" {
		return nil, http.ErrNotMultipart
	}

	d, params, err := mime.ParseMediaType(v)
	if err != nil || !strings.HasPrefix(d, "multipart/") {
		return nil, http.ErrNotMultipart
	}

	boundary, ok := params["boundary"]
	if !ok {
		return nil, http.ErrMissingBoundary
	}

	return multipart.NewReader(req.Body, boundary), nil
}

// Done returns wither the context is marked as done or not.
func (ctx *Context) Done() bool { return ctx.done }

// SetCookie sets an http-only cookie using the passed name, value and domain.
// Returns an error if there was a problem encoding the value.
// if forceSecure is true, it will set the Secure flag to true, otherwise it sets it based on the connection.
// if duration == -1, it sets expires to 10 years in the past, if 0 it gets ignored (aka session-only cookie),
// if duration > 0, the expiration date gets set to now() + duration.
// Note that for more complex options, you can use http.SetCookie(ctx, &http.Cookie{...}).
func (ctx *Context) SetCookie(name string, value any, domain string, forceHTTPS bool, duration time.Duration) (err error) {
	var encValue string
	if sc := GetSecureCookie(ctx); sc != nil {
		if encValue, err = sc.Encode(name, value); err != nil {
			return
		}
	} else if s, ok := value.(string); ok {
		encValue = s
	} else {
		var j []byte
		if j, err = internal.Marshal(value); err != nil {
			return
		}
		encValue = string(j)
	}

	cookie := &http.Cookie{
		Path:     "/",
		Name:     name,
		Value:    encValue,
		Domain:   domain,
		HttpOnly: true,
		Secure:   forceHTTPS || ctx.Req.TLS != nil,
	}

	switch duration {
	case 0: // session only
	case -1:
		cookie.Expires = nukeCookieDate
	default:
		cookie.Expires = time.Now().UTC().Add(duration)

	}

	http.SetCookie(ctx, cookie)
	return
}

// RemoveCookie deletes the given cookie and sets its expires date in the past.
func (ctx *Context) RemoveCookie(name string) {
	http.SetCookie(ctx, &http.Cookie{
		Path:     "/",
		Name:     name,
		Value:    "::deleted::",
		HttpOnly: true,
		Expires:  nukeCookieDate,
	})
}

// GetCookie returns the given cookie's value.
func (ctx *Context) GetCookie(name string) (out string, ok bool) {
	c, err := ctx.Req.Cookie(name)
	if err != nil {
		return
	}
	if sc := GetSecureCookie(ctx); sc != nil {
		ok = sc.Decode(name, c.Value, &out) == nil
		return
	}
	return c.Value, true
}

// GetCookieValue unmarshals a cookie, only needed if you stored an object for the cookie not a string.
func (ctx *Context) GetCookieValue(name string, valDst any) error {
	c, err := ctx.Req.Cookie(name)
	if err != nil {
		return err
	}

	if sc := GetSecureCookie(ctx); sc != nil {
		return sc.Decode(name, c.Value, valDst)
	}

	return internal.UnmarshalString(c.Value, valDst)
}

func (ctx *Context) Logf(format string, v ...any) {
	ctx.s.logfStack(1, format, v...)
}

func (ctx *Context) LogSkipf(skip int, format string, v ...any) {
	ctx.s.logfStack(skip+1, format, v...)
}

var ctxPool = sync.Pool{
	New: func() any {
		return &Context{
			data: M{},
		}
	},
}

func getCtx(rw http.ResponseWriter, req *http.Request, p router.Params, s *Server) *Context {
	ctx := ctxPool.Get().(*Context)
	if strings.Contains(req.Header.Get(acceptHeader), gzEnc) {
		rw = getGzipRW(rw)
	}

	var q url.Values
	if rq := req.URL.RawQuery; rq != "" {
		q, _ = url.ParseQuery(rq)
	}

	*ctx = Context{
		ResponseWriter: rw,

		Req: req,
		s:   s,

		data: ctx.data,

		Params:   p,
		ReqQuery: q,
	}

	return ctx
}

func putCtx(ctx *Context) {
	if g, ok := ctx.ResponseWriter.(*gzipRW); ok {
		g.Reset()
	}

	m := ctx.data

	// this looks like a bad idea, but it's an optimization in go 1.11, minor perf hit on 1.10
	for k := range m {
		delete(m, k)
	}

	*ctx = Context{
		data: m,
	}

	ctxPool.Put(ctx)
}
