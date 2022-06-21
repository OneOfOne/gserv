package router

import (
	"fmt"
	"net/http"
	"runtime/pprof"
	"time"
)

// Handler is what handler looks like, duh?
// *note* `p` is NOT safe to be used outside the handler, call p.Copy() if you need to use it.
type Handler = func(w http.ResponseWriter, req *http.Request, p Params)

// PanicHandler is a special handler that gets called if a panic happens
type PanicHandler = func(w http.ResponseWriter, req *http.Request, v any)

// DefaultPanicHandler is the default panic handler
func DefaultPanicHandler(w http.ResponseWriter, req *http.Request, v any) {
	http.Error(w, fmt.Sprintf("panic (%T): %v", v, v), http.StatusInternalServerError)
}

// DefaultNotFoundHandler is the default panic handler
func DefaultNotFoundHandler(w http.ResponseWriter, req *http.Request, _ Params) {
	http.Error(w, "404 page not found", http.StatusNotFound)
}

// ServeHTTP implements http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	start := time.Now()

	if r.opts.CatchPanics && r.PanicHandler != nil {
		defer func() {
			if v := recover(); v != nil {
				r.PanicHandler(w, req, v)
			}
		}()
	}

	u, method := req.URL.Path, req.Method

	if !r.opts.NoAutoCleanURL {
		var ok bool
		if u, ok = cleanPath(u); ok {
			req.URL.Path = u
		}
	}

	if method == http.MethodHead && !r.opts.NoAutoHeadToGet {
		w, method = &headRW{ResponseWriter: w}, http.MethodGet
	}

	if g, h, p := r.match(method, pathNoQuery(u)); h != nil {
		if r.opts.ProfileLabels {
			labels := pprof.Labels("group", g, "method", req.Method, "uri", req.RequestURI)
			ctx := pprof.WithLabels(req.Context(), labels)
			pprof.SetGoroutineLabels(ctx)
			req = req.WithContext(ctx)
		}

		h(w, req, p.Params())
		r.putParams(p)

		if r.opts.OnRequestDone != nil {
			r.opts.OnRequestDone(req.Context(), g, method, u, time.Since(start))
		}

		return
	}

	if method == http.MethodGet {
		if r.NotFoundHandler != nil {
			r.NotFoundHandler(w, req, nil)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	} else {
		if r.MethodNotAllowedHandler != nil {
			r.MethodNotAllowedHandler(w, req, nil)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}
