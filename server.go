package gserv

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.oneofone.dev/gserv/router"
	"go.oneofone.dev/oerrs"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

var DefaultPanicHandler = func(ctx *Context, v any, fr *oerrs.Frame) {
	resp := NewJSONErrorResponse(500, fmt.Sprintf("PANIC in %s %s: %v", ctx.Req.Method, ctx.Path(), v), fmt.Sprintf("at %s %s:%d", fr.Function, fr.File, fr.Line))
	ctx.Encode(nil, 500, resp)
}

var noopLogger = log.New(io.Discard, "", 0)

// DefaultOpts are the default options used for creating new servers.
var DefaultOpts = Options{
	WriteTimeout: time.Minute,
	ReadTimeout:  time.Minute,

	MaxHeaderBytes: 1 << 20, // 1MiB

	Logger: log.New(os.Stderr, "gserv: ", 0),
}

// New returns a new server with the specified options.
func New(opts ...Option) *Server {
	o := DefaultOpts

	for _, opt := range opts {
		opt(&o)
	}

	return NewWithOpts(&o)
}

// NewWithOpts allows passing the Options struct directly
func NewWithOpts(opts *Options) *Server {
	srv := &Server{}

	if opts == nil {
		cp := DefaultOpts
		srv.opts = cp
	} else {
		srv.opts = *opts
	}

	ro := srv.opts.RouterOptions
	srv.r = router.New(ro)

	if srv.opts.CatchPanics {
		srv.PanicHandler = DefaultPanicHandler
	}

	srv.r.NotFoundHandler = func(w http.ResponseWriter, req *http.Request, p router.Params) {
		if h := srv.NotFoundHandler; h != nil {
			ctx := getCtx(w, req, p, srv)
			srv.NotFoundHandler(ctx)
			putCtx(ctx)
			return
		}

		RespNotFound.WriteToCtx(&Context{
			Req:            req,
			ResponseWriter: w,
		})
	}

	srv.s = srv

	return srv
}

type (
	PanicHandler = func(ctx *Context, v any, fr *oerrs.Frame)
)

// Server is the main server
type Server struct {
	Group
	r *router.Router

	PanicHandler
	NotFoundHandler func(ctx *Context)

	servers    []*http.Server
	opts       Options
	serversMux sync.Mutex
	closed     int32
}

// ServeHTTP allows using the server in custom scenarios that expects an http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.r.ServeHTTP(w, req)
}

func (s *Server) newHTTPServer(ctx context.Context, addr string, forceHTTP2 bool) *http.Server {
	opts := &s.opts

	h := http.Handler(s.r)
	if forceHTTP2 {
		h = h2c.NewHandler(s.r, &http2.Server{})
	}

	lg := opts.Logger
	if !opts.EnableDefaultHTTPLogging {
		lg = noopLogger
	}

	return &http.Server{
		Addr:    addr,
		Handler: h,

		ReadTimeout:    opts.ReadTimeout,
		WriteTimeout:   opts.WriteTimeout,
		MaxHeaderBytes: opts.MaxHeaderBytes,
		ErrorLog:       lg,

		BaseContext: func(net.Listener) context.Context { return ctx },
	}
}

// Run starts the server on the specific address
func (s *Server) Run(ctx context.Context, addr string) error {
	if addr == "" {
		addr = ":http"
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	srv := s.newHTTPServer(ctx, ln.Addr().String(), true)

	s.serversMux.Lock()
	s.servers = append(s.servers, srv)
	s.serversMux.Unlock()

	return srv.Serve(ln)
}

// CertPair is a pair of (cert, key) files to listen on TLS
type CertPair struct {
	CertFile string `json:"certFile"`
	KeyFile  string `json:"KeyFile"`
}

// SetKeepAlivesEnabled controls whether HTTP keep-alives are enabled.
// By default, keep-alives are always enabled.
func (s *Server) SetKeepAlivesEnabled(v bool) {
	s.serversMux.Lock()
	for _, srv := range s.servers {
		srv.SetKeepAlivesEnabled(v)
	}
	s.serversMux.Unlock()
}

// Addrs returns all the listening addresses used by the underlying http.Server(s).
func (s *Server) Addrs() (out []string) {
	s.serversMux.Lock()
	out = make([]string, len(s.servers))
	for i, srv := range s.servers {
		out[i] = srv.Addr
	}
	s.serversMux.Unlock()
	return
}

// Closed returns true if the server is already shutdown/closed
func (s *Server) Closed() bool {
	return atomic.LoadInt32(&s.closed) == 1
}

// Logf logs to the default server logger if set
func (s *Server) Logf(f string, args ...any) {
	s.logfStack(2, f, args...)
}

func (s *Server) logfStack(n int, f string, args ...any) {
	lg := s.opts.Logger
	if lg == nil {
		lg = log.Default()
	}

	_, file, line, ok := runtime.Caller(n + 1)
	if !ok {
		file = "???"
		line = 0
	}

	// make it output the package owning the file
	parts := strings.Split(file, "/")
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}

	lg.Printf(strings.Join(parts, "/")+":"+strconv.Itoa(line)+": "+f, args...)
}

// AllowCORS is an alias for s.AddRoute("OPTIONS", path, AllowCORS(allowedMethods...))
func (s *Server) AllowCORS(path string, allowedMethods ...string) {
	s.AddRoute(http.MethodOptions, path, AllowCORS(allowedMethods, nil, nil))
}

func (s *Server) Swagger() *router.Swagger {
	return s.r.Swagger()
}
