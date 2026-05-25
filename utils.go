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
	"go.oneofone.dev/otk"
	"golang.org/x/net/http2"
)

var nukeCookieDate = time.Date(1991, time.August, 6, 0, 0, 0, 0, time.UTC)

// HTTPHandler converts an http.Handler into a gserv Handler.
func HTTPHandler(h http.Handler) Handler {
	return HTTPHandlerFunc(h.ServeHTTP)
}

// HTTPHandlerFunc converts an http.HandlerFunc into a gserv Handler.
func HTTPHandlerFunc(h http.HandlerFunc) Handler {
	return func(ctx *Context) Response {
		h(ctx, ctx.Req)
		return nil
	}
}

// StaticDirStd returns a handler that serves static files from the given directory with an optional prefix.
// If allowListing is false, directory listing is disabled and index.html is served for directories.
func StaticDirStd(prefix, dir string, allowListing bool) Handler {
	var fs http.FileSystem
	if allowListing {
		fs = http.Dir(dir)
	} else {
		fs = noListingDir(dir)
	}
	return HTTPHandler(http.StripPrefix(prefix, http.FileServer(fs)))
}

// StaticDir returns a handler that serves static files from the given directory without prefix or listing.
func StaticDir(dir, paramName string) Handler {
	return StaticDirStd("", dir, false)
	// return StaticDirWithLimit(dir, paramName, -1)
}

// StaticDirWithLimit returns a handler that serves static files from the given directory.
// The paramName is the path param used to extract the file path, for example: s.GET("/s/*fp", StaticDirWithLimit("./static/", "fp", 1000)).
// If limit is greater than 0, at most N requests can be served simultaneously.
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
		return f, err
	}

	if s, _ := f.Stat(); s != nil && s.IsDir() {
		f.Close()
		index := strings.TrimSuffix(name, "/") + "/index.html"
		return hd.Open(index)
	}

	return f, err
}

func matchStarOrigin(set otk.Set, keys []string, origin string) bool {
	origin = strings.TrimPrefix(origin, "https://")
	if set.Has(origin) {
		return true
	}

	if strings.Count(origin, ".") < 2 {
		return false
	}

	_, origin, found := strings.Cut(origin, ".")
	if !found {
		return false
	}

	for _, orig := range keys {
		orig, found := strings.CutPrefix(orig, "*.")
		if !found {
			continue
		}
		if strings.HasSuffix(origin, orig) {
			return true
		}

	}
	return false
}

// AllowCORS returns a CORS middleware that allows cross-origin requests.
// If methods is empty, the requested method from Access-Control-Request-Method is used.
// If headers is empty, the requested headers from Access-Control-Request-Headers are used.
// If origins is empty, any origin is allowed via wildcard matching for subdomains.
// It automatically installs an OPTIONS preflight handler to each passed group.
func AllowCORS(methods, headers, origins []string, groups ...GroupType) Handler {
	ms := strings.Join(methods, ", ")
	hs := strings.Join(headers, ", ")

	om := otk.NewSet()
	for _, orig := range origins {
		om.Set(strings.TrimPrefix(orig, "https://"))
	}
	omKeys := om.Keys()

	fn := func(ctx *Context) (_ Response) {
		rh, wh := ctx.Req.Header, ctx.Header()
		origin := rh.Get("Origin")

		if origin == "" { // return early if it's not a browser request
			return
		}

		if len(om) == 0 || matchStarOrigin(om, omKeys, origin) {
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

// M is a shorthand type for map[string]any, used for context values.
type M map[string]any

// ToJSON returns a JSON string representation of M, primarily for debugging.
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

// MultiError accumulates multiple errors and can be returned as a single error.
type MultiError []error

// Push adds an error to the MultiError slice if err is not nil.
func (me *MultiError) Push(err error) {
	if err != nil {
		*me = append(*me, err)
	}
}

// Err returns nil if there are no accumulated errors, or a combined error otherwise.
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

// H2Client returns an HTTP client configured for HTTP/2 with cleartext (h2c) support.
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

// DummyResponseWriter is a response writer that buffers output for inspection.
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
