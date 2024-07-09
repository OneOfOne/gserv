package gserv

import (
	"bytes"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.oneofone.dev/gserv/internal"
	"golang.org/x/net/http2"
)

var nukeCookieDate = time.Date(1991, time.August, 6, 0, 0, 0, 0, time.UTC)

// HTTPHandler returns a Handler from an http.Handler.
func HTTPHandler(h http.Handler) Handler {
	return HTTPHandlerFunc(h.ServeHTTP)
}

// FromHTTPHandlerFunc returns a Handler from an http.Handler.
func HTTPHandlerFunc(h http.HandlerFunc) Handler {
	return func(ctx *Context) Response {
		h(ctx, ctx.Req)
		return nil
	}
}

// StaticDirStd is a QoL wrapper for http.FileServer(http.Dir(dir)).
func StaticDirStd(prefix, dir string, allowListing bool) Handler {
	var fs http.FileSystem
	if allowListing {
		fs = http.Dir(dir)
	} else {
		fs = noListingDir(dir)
	}
	return HTTPHandler(http.StripPrefix(prefix, http.FileServer(fs)))
}

// StaticDir is a shorthand for StaticDirWithLimit(dir, paramName, -1).
func StaticDir(dir, paramName string) Handler {
	return StaticDirStd("", dir, false)
	// return StaticDirWithLimit(dir, paramName, -1)
}

// StaticDirWithLimit returns a handler that handles serving static files.
// paramName is the path param, for example: s.GET("/s/*fp", StaticDirWithLimit("./static/", "fp", 1000)).
// if limit is > 0, it will only ever serve N files at a time.
// BUG: returns 0 size for some reason
func StaticDirWithLimit(dir, paramName string, limit int) Handler {
	var (
		sem chan struct{}
		e   struct{}
	)

	if limit > 0 {
		sem = make(chan struct{}, limit)
	}

	return func(ctx *Context) Response {
		path := ctx.Param(paramName)
		if sem != nil {
			sem <- e
			defer func() { <-sem }()
		}

		if err := ctx.File(filepath.Join(dir, path)); err != nil {
			if os.IsNotExist(err) {
				http.Error(ctx, "file not found", http.StatusNotFound)
				return nil
			}
			http.Error(ctx, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}
}

type noListingDir string

func (d noListingDir) Open(name string) (f http.File, err error) {
	hd := http.Dir(d)

	if f, err = hd.Open(name); err != nil {
		return
	}

	if s, _ := f.Stat(); s != nil && s.IsDir() {
		f.Close()
		index := strings.TrimSuffix(name, "/") + "/index.html"
		return hd.Open(index)
	}

	return
}

// AllowCORS allows CORS responses.
// If methods is empty, it will respond with the requested method.
// If headers is empty, it will respond with the requested headers.
// If origins is empty, it will respond with the requested origin.
// will automatically install an OPTIONS handler to each passed group.
func AllowCORS(methods, headers, origins []string, groups ...GroupType) Handler {
	ms := strings.Join(methods, ", ")
	hs := strings.Join(headers, ", ")

	om := map[string]bool{}
	for _, orig := range origins {
		om[orig] = true
	}

	fn := func(ctx *Context) (_ Response) {
		rh, wh := ctx.Req.Header, ctx.Header()
		origin := rh.Get("Origin")

		if origin == "" { // return early if it's not a browser request
			return
		}

		if len(om) == 0 || om[origin] {
			wh.Set("Access-Control-Allow-Origin", origin)
			wh.Set("Access-Control-Allow-Credentials", "true")
		} else {
			return
		}

		if len(ms) > 0 {
			wh.Set("Access-Control-Allow-Methods", ms)
		} else if rm := rh.Get("Access-Control-Request-Method"); rm != "" {
			wh.Set("Access-Control-Allow-Methods", rm)
		}

		if len(hs) > 0 {
			wh.Set("Access-Control-Allow-Headers", hs)
		} else if rh := rh.Get("Access-Control-Request-Headers"); rh != "" {
			wh.Set("Access-Control-Allow-Headers", rh)
		}

		wh.Set("Access-Control-Max-Age", "86400") // 24 hours

		return
	}

	for _, g := range groups {
		g.AddRoute("OPTIONS", "/*x", fn)
	}

	return fn
}

type M map[string]any

// ToJSON returns a string json representation of M, mostly for debugging.
func (m M) ToJSON(indent bool) string {
	if len(m) == 0 {
		return "{}"
	}
	var j []byte

	if indent {
		j, _ = internal.MarshalIndent(m)
	} else {
		j, _ = internal.Marshal(m)
	}
	return string(j)
}

// MultiError handles returning multiple errors.
type MultiError []error

// Push adds an error to the MultiError slice if err != nil.
func (me *MultiError) Push(err error) {
	if err != nil {
		*me = append(*me, err)
	}
}

// Err returns nil if me is empty.
func (me MultiError) Err() error {
	if len(me) == 0 {
		return nil
	}

	if len(me) == 1 {
		return me[0]
	}

	return me
}

func (me MultiError) Error() string {
	errs := make([]string, 0, len(me))
	for _, err := range me {
		errs = append(errs, err.Error())
	}

	return "multiple errors returned:\n\t" + strings.Join(errs, "\n\t")
}

func H2Client() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
}

type DummyResponseWriter struct {
	h   http.Header
	buf bytes.Buffer
	st  int
}

func (d *DummyResponseWriter) Header() http.Header {
	if d.h == nil {
		d.h = make(http.Header)
	}
	return d.h
}

func (d *DummyResponseWriter) Write(b []byte) (int, error) {
	if d.buf.Len() == 0 {
		d.WriteHeader(200)
	}
	return d.buf.Write(b)
}

func (d *DummyResponseWriter) WriteHeader(v int) {
	d.st = v
	d.h.Write(&d.buf)
}

func (d *DummyResponseWriter) Status() int {
	return d.st
}

func (d *DummyResponseWriter) Bytes() []byte {
	return d.buf.Bytes()
}
