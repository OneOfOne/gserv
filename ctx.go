package gserv

import (
	"errors"
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
	// ErrDir is returned from ctx.File when the path is a directory, not a file.
	ErrDir = oerrs.String("file is a directory")

	// ErrInvalidURL is returned on invalid redirect URLs.
	ErrInvalidURL = oerrs.String("invalid redirect error")

	// ErrEmptyCallback is returned when a callback is empty.
	ErrEmptyCallback = oerrs.String("empty callback")

	// ErrEmptyData is returned when the data payload is empty.
	ErrEmptyData = oerrs.String("payload data is empty")
)

// Context is the default context passed to handlers. It is not thread safe and should never be used outside the handler.
type Context struct {
	http.ResponseWriter
	Codec Codec

	nextMW       func()
	s            *Server
	data         M
	Req          *http.Request
	next         func()
	ReqQuery     url.Values
	Params       router.Params
	bytesWritten int
	status       int

	hijackServeContent bool
	done               bool
}

// Route returns the current route.
func (ctx *Context) Route() *router.Route {
	return router.RouteFromRequest(ctx.Req)
}

// Param returns a path parameter by key name.
func (ctx *Context) Param(key string) string {
	return ctx.Params.Get(key)
}

// Query returns a query parameter value by key name.
func (ctx *Context) Query(key string) string {
	return ctx.ReqQuery.Get(key)
}

// QueryDefault returns the query parameter value for key, or the default value if missing.
func (ctx *Context) QueryDefault(key, def string) string {
	if v := ctx.ReqQuery.Get(key); v != "" {
		return v
	}
	return def
}

// Get retrieves a value stored in the context by key.
func (ctx *Context) Get(key string) any {
	return ctx.data[key]
}

// Set stores a value in the context under the given key, useful for passing data to other handlers down the chain.
func (ctx *Context) Set(key string, val any) {
	if ctx.data == nil {
		ctx.data = make(M)
	}
	ctx.data[key] = val
}

// WriteReader writes the data from the given reader to the response with an optional content-type.
func (ctx *Context) WriteReader(contentType string, r io.Reader) (int64, error) {
	if contentType != "" {
		ctx.SetContentType(contentType)
	}

	return io.Copy(ctx, r)
}

// File serves a file using http.ServeContent. See http.ServeContent.
func (ctx *Context) File(fp string) error {
	ctx.hijackServeContent = true
	http.ServeFile(ctx, ctx.Req, fp)

	return nil
}

// Path returns the escaped path of the current request.
func (ctx *Context) Path() string {
	return ctx.Req.URL.EscapedPath()
}

// SetContentType sets the response's content-type header.
func (ctx *Context) SetContentType(typ string) {
	if typ == "" {
		return
	}
	h := ctx.Header()
	h.Set(contentTypeHeader, typ)
}

// ReqHeader returns a request header value by key name.
func (ctx *Context) ReqHeader(key string) string {
	return ctx.Req.Header.Get(key)
}

// ContentType returns the request's content-type.
func (ctx *Context) ContentType() string {
	return ctx.ReqHeader(contentTypeHeader)
}

// Read reads from the request body, implementing io.Reader.
func (ctx *Context) Read(p []byte) (int, error) {
	return ctx.Req.Body.Read(p)
}

// CloseBody closes the request body.
func (ctx *Context) CloseBody() error {
	return ctx.Req.Body.Close()
}

// BindJSON parses the request's body as JSON and closes the body.
// Note that unlike gin.Context.Bind, this does NOT verify the fields using special tags.
func (ctx *Context) BindJSON(out any) error {
	return ctx.BindCodec(JSONCodec{}, out)
}

// BindMsgpack parses the request's body as msgpack and closes the body.
// Note that unlike gin.Context.Bind, this does NOT verify the fields using special tags.
func (ctx *Context) BindMsgpack(out any) error {
	return ctx.BindCodec(MsgpCodec{}, out)
}

// BindCodec parses the request's body using the given codec and closes the body.
// Note that unlike gin.Context.BindCodec, this does NOT verify the fields using special tags.
func (ctx *Context) BindCodec(c Codec, out any) error {
	c = genh.FirstNonZero(c, ctx.Codec, DefaultCodec)
	err := c.Decode(ctx, out)
	_ = ctx.CloseBody()
	if errors.Is(err, io.EOF) {
		return ErrEmptyData
	}
	return err
}

// Bind parses the request's body based on its content type and closes the body.
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
	_ = ctx.CloseBody()
	if err != nil {
		err = oerrs.Errorf("error decoding (%s): %w", ct, err)
	}
	return err
}

// Printf writes a formatted string to the response with the given status code and content type.
// Calling this function marks the Context as done, meaning any returned responses won't be written out.
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

// JSON encodes data as JSON and writes it to the response with the given status code.
// Calling this function marks the Context as done, meaning any returned responses won't be written out.
func (ctx *Context) JSON(code int, indent bool, v any) error {
	return ctx.EncodeCodec(JSONCodec{indent}, code, v)
}

// Msgpack encodes data as msgpack and writes it to the response with the given status code.
// Calling this function marks the Context as done, meaning any returned responses won't be written out.
func (ctx *Context) Msgpack(code int, v any) error {
	return ctx.EncodeCodec(MsgpCodec{}, code, v)
}

// EncodeCodec encodes data using the given codec and writes it to the response with the given status code.
func (ctx *Context) EncodeCodec(c Codec, code int, v any) error {
	c = genh.FirstNonZero(c, ctx.Codec, DefaultCodec)
	ctx.done = true
	ctx.SetContentType(c.ContentType())

	if code > 0 {
		ctx.WriteHeader(code)
	}
	return c.Encode(ctx, v)
}

// Encode encodes data using the content type of the request and writes it to the response with the given status code.
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

// ClientIP returns the client's IP address, accounting for X-Real-Ip and X-Forwarded-For headers.
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

// NextMiddleware executes all remaining middlewares in the group, returning before the handlers run.
// It will panic if called from a handler.
func (ctx *Context) NextMiddleware() {
	if ctx.nextMW != nil {
		ctx.nextMW()
	}
}

// NextHandler executes all remaining handlers in the group up until one returns a Response.
func (ctx *Context) NextHandler() {
	if ctx.next != nil {
		ctx.next()
	}
}

// Next executes NextMiddleware() then NextHandler() if NextMiddleware() didn't return a response.
func (ctx *Context) Next() {
	ctx.NextMiddleware()
	ctx.NextHandler()
}

// WriteHeader writes the HTTP status code to the response.
func (ctx *Context) WriteHeader(s int) {
	if ctx.status = s; ctx.hijackServeContent && ctx.status >= http.StatusBadRequest {
		return
	}

	ctx.ResponseWriter.WriteHeader(s)
}

// Write writes bytes to the response, implementing http.ResponseWriter.
func (ctx *Context) Write(p []byte) (int, error) {
	if ctx.hijackServeContent && ctx.status >= http.StatusBadRequest {
		ctx.hijackServeContent = false
		return len(p), NewJSONErrorResponse(ctx.status, p).WriteToCtx(ctx)
	}

	ctx.done = true
	ctx.bytesWritten += len(p)
	return ctx.ResponseWriter.Write(p)
}

// LimitRead limits the request body to the given size.
func (ctx *Context) LimitRead(sz int64) {
	ctx.Req.Body = http.MaxBytesReader(ctx, ctx.Req.Body, sz)
}

// BytesWritten returns the number of bytes written to the response body.
func (ctx *Context) BytesWritten() int {
	return ctx.bytesWritten
}

// WriteString writes a string to the response, implementing io.StringWriter.
func (ctx *Context) WriteString(p string) (int, error) {
	return ctx.ResponseWriter.Write(otk.UnsafeBytes(p))
}

// Flush flushes any buffered data to the connection, implementing http.Flusher.
func (ctx *Context) Flush() {
	if f, ok := ctx.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Status returns the last HTTP status code written via WriteHeader.
func (ctx *Context) Status() int {
	if ctx.status == 0 {
		ctx.status = http.StatusOK
	}

	return ctx.status
}

// MultipartReader parses the request as a multipart body and returns a reader for its parts.
// Unlike Request.MultipartReader, it supports multipart/* content types, not just form-data.
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

// Finished returns true if the context has been marked as done.
func (ctx *Context) Finished() bool {
	return ctx.done
}

// SetCookie sets an HTTP-only cookie with the given name, value, domain, and secure flag.
// Returns an error if there was a problem encoding the value.
// If forceSecure is true, it sets the Secure flag; otherwise it sets it based on the connection (TLS).
// If duration == -1, it sets expires to 10 years in the past. If duration == 0, it is ignored (session-only cookie).
// If duration > 0, the expiration date is set to now() + duration.
func (ctx *Context) SetCookie(name string, value any, domain string, forceHTTPS bool, duration time.Duration) (err error) {
	var encValue string
	if sc := GetSecureCookie(ctx); sc != nil {
		if encValue, err = sc.Encode(name, value); err != nil {
			return err
		}
	} else if s, ok := value.(string); ok {
		encValue = s
	} else {
		var j []byte
		if j, err = internal.Marshal(value); err != nil {
			return err
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
	return err
}

// RemoveCookie deletes the given cookie by setting its expiration date in the past.
func (ctx *Context) RemoveCookie(name string) {
	http.SetCookie(ctx, &http.Cookie{
		Path:     "/",
		Name:     name,
		Value:    "::deleted::",
		HttpOnly: true,
		Expires:  nukeCookieDate,
	})
}

// GetCookie retrieves the value of a cookie by name, returning it and whether it was found.
func (ctx *Context) GetCookie(name string) (out string, ok bool) {
	c, err := ctx.Req.Cookie(name)
	if err != nil {
		return out, ok
	}
	if sc := GetSecureCookie(ctx); sc != nil {
		ok = sc.Decode(name, c.Value, &out) == nil
		return out, ok
	}
	return c.Value, true
}

// GetCookieValue retrieves and unmarshals a cookie value into the given destination.
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

// Logf logs a formatted message using the server's logger.
func (ctx *Context) Logf(format string, v ...any) {
	ctx.s.logfStack(1, format, v...)
}

// LogSkipf logs a formatted message with the given skip frame offset for caller info.
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
	if !s.NoCompression && strings.Contains(req.Header.Get(acceptHeader), gzEnc) {
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
