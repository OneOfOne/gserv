// Package gserv is a simple, fast, and flexible HTTP server framework for Go.
package gserv

import (
	"context"
	"errors"
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

// DefaultPanicHandler is the default panic recovery handler that logs the panic and returns a JSON 500 error response.
var DefaultPanicHandler = func(ctx *Context, v any, fr *oerrs.Frame) {
	msg, info := fmt.Sprintf("PANIC in %s %s: %v", ctx.Req.Method, ctx.Path(), v), fmt.Sprintf("at %s %s:%d", fr.Function, fr.File, fr.Line)
	ctx.Logf("%s (%s)", msg, info)
	resp := NewJSONErrorResponse(500, "internal server error")
	_ = ctx.Encode(500, resp)
}

var noopLogger = log.New(io.Discard, "", 0)

// DefaultOpts are the default options used for creating new servers.
var DefaultOpts = Options{
	WriteTimeout: time.Minute,
	ReadTimeout:  time.Minute,

	MaxHeaderBytes: 1 << 20, // 1MiB

	Logger: log.New(os.Stderr, "gserv: ", 0),
}

// New creates a new Server with the given options.
func New(opts ...Option) *Server {
	o := DefaultOpts

	for _, opt := range opts {
		opt(&o)
	}

	return NewWithOpts(&o)
}

// NewWithOpts creates a new Server from an Options struct.
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

		_ = RespNotFound.WriteToCtx(&Context{
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

// Server is the main HTTP server type.
type Server struct {
	Group
	r *router.Router

	PanicHandler
	NotFoundHandler func(ctx *Context)

	servers    []*http.Server
	opts       Options
	serversMux sync.Mutex
	closed     int32

	NoCompression bool // used by proxies
}

// ServeHTTP implements http.Handler, allowing the server to be used in custom scenarios.
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
	// if !opts.EnableDefaultHTTPLogging {
	// 	lg = noopLogger
	// }

	srv := &http.Server{
		Addr:    addr,
		Handler: h,

		ReadTimeout:    opts.ReadTimeout,
		WriteTimeout:   opts.WriteTimeout,
		MaxHeaderBytes: opts.MaxHeaderBytes,
		ErrorLog:       lg,

		BaseContext: func(net.Listener) context.Context { return ctx },
		ConnContext: func(context.Context, net.Conn) context.Context { return ctx },
	}

	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(ctx)
	}()

	return srv
}

// Run starts the server on the given address with HTTP/2 support.
// If addr is empty, it defaults to ":http".
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

	if err = srv.Serve(ln); err != nil && errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}

// SetKeepAlivesEnabled enables or disables HTTP keep-alives on all underlying servers.
func (s *Server) SetKeepAlivesEnabled(v bool) {
	s.serversMux.Lock()
	for _, srv := range s.servers {
		srv.SetKeepAlivesEnabled(v)
	}
	s.serversMux.Unlock()
}

// Addrs returns all the listening addresses of the underlying http.Servers.
func (s *Server) Addrs() (out []string) {
	s.serversMux.Lock()
	out = make([]string, len(s.servers))
	for i, srv := range s.servers {
		out[i] = srv.Addr
	}
	s.serversMux.Unlock()
	return out
}

// Closed returns true if the server has been shut down or closed.
func (s *Server) Closed() bool {
	return atomic.LoadInt32(&s.closed) == 1
}

// Logf logs a formatted message using the server's logger.
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

// AllowCORS adds an OPTIONS route for CORS support using the given allowed methods.
func (s *Server) AllowCORS(path string, allowedMethods ...string) {
	s.AddRoute(http.MethodOptions, path, AllowCORS(allowedMethods, nil, nil))
}

// Swagger returns the swagger documentation instance for this server.
func (s *Server) Swagger() *router.Swagger {
	return s.r.Swagger()
}

// Close immediately closes all the active underlying http servers and connections without graceful shutdown.
func (s *Server) Close() error {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return http.ErrServerClosed
	}

	var me MultiError
	s.serversMux.Lock()
	for _, srv := range s.servers {
		srv.SetKeepAlivesEnabled(false)
		if err := srv.Close(); err != nil {
			err = fmt.Errorf("%s (%T): %s", srv.Addr, err, err)
			me.Push(err)
		}
	}

	s.servers = nil
	s.serversMux.Unlock()

	return me.Err()
}

// Shutdown gracefully shuts down all the underlying http servers, optionally with a timeout.
func (s *Server) Shutdown(timeout time.Duration) error {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return http.ErrServerClosed
	}

	var (
		me  MultiError
		ctx = context.Background()
	)

	if timeout > 0 {
		var cancelFn func()
		ctx, cancelFn = context.WithDeadline(ctx, time.Now().Add(timeout))
		defer cancelFn()
	}

	s.serversMux.Lock()
	for _, srv := range s.servers {
		srv.SetKeepAlivesEnabled(false)
		me.Push(srv.Shutdown(ctx))
	}
	s.servers = nil
	s.serversMux.Unlock()

	return me.Err()
}
