package gserv

import (
	"net/http"
	"strings"

	"go.oneofone.dev/gserv/router"
	"go.oneofone.dev/oerrs"
)

type (
	// Route is a type alias for the router's Route type.
	Route = *router.Route
)

var DefaultCodec Codec = &JSONCodec{} // DefaultCodec is the default codec used by the framework, set to JSONCodec.

// Handler is the default server handler type. In a handler chain, returning a non-nil Response breaks the chain.
type Handler = func(ctx *Context) Response

// Group is a collection of routes with shared middleware and path prefix.
type Group struct {
	s    *Server
	nm   string
	path string
	mw   []Handler
}

// Use adds more middleware to the current group.
func (g *Group) Use(mw ...Handler) {
	g.mw = append(g.mw, mw...)
}

// Routes returns all registered routes. Each route is returned as [group name, method, path].
func (g *Group) Routes() [][3]string {
	return g.s.r.GetRoutes()
}

// AddRoute adds one or more handlers for the given HTTP method and path to this group.
// It is NOT safe to call this after starting the server.
func (g *Group) AddRoute(method, path string, handlers ...Handler) Route {
	ghc := groupHandlerChain{
		hc: handlers,
		g:  g,
	}
	p := joinPath(g.path, path)
	return g.s.r.AddRoute(g.nm, method, p, ghc.Serve)
}

// GET registers a GET route for the given path with the specified handlers.
func (g *Group) GET(path string, handlers ...Handler) Route {
	return g.AddRoute(http.MethodGet, path, handlers...)
}

// PUT registers a PUT route for the given path with the specified handlers.
func (g *Group) PUT(path string, handlers ...Handler) Route {
	return g.AddRoute(http.MethodPut, path, handlers...)
}

// POST registers a POST route for the given path with the specified handlers.
func (g *Group) POST(path string, handlers ...Handler) Route {
	return g.AddRoute(http.MethodPost, path, handlers...)
}

// DELETE registers a DELETE route for the given path with the specified handlers.
func (g *Group) DELETE(path string, handlers ...Handler) Route {
	return g.AddRoute(http.MethodDelete, path, handlers...)
}

// OPTIONS registers an OPTIONS route for the given path with the specified handlers.
func (g *Group) OPTIONS(path string, handlers ...Handler) Route {
	return g.AddRoute(http.MethodOptions, path, handlers...)
}

func (g *Group) DisableRoute(method, path string, disabled bool) bool {
	return g.s.r.DisableRoute(method, joinPath(g.path, path), disabled)
}

// Static registers a GET route that serves static files from the given local path.
func (g *Group) Static(path, localPath string, allowListing bool) Route {
	path = strings.TrimSuffix(path, "/")

	return g.AddRoute(http.MethodGet, joinPath(path, "*fp"), StaticDirStd(path, localPath, allowListing))
}

// StaticFile registers a GET route that serves a single static file.
func (g *Group) StaticFile(path, localPath string) Route {
	return g.AddRoute(http.MethodGet, path, func(ctx *Context) Response {
		_ = ctx.File(localPath)
		return nil
	})
}

// SubGroup creates a new sub-group inheriting the current group's middleware.
func (g *Group) SubGroup(name, path string, mw ...Handler) *Group {
	return &Group{
		nm:   name,
		mw:   append(g.mw[:len(g.mw):len(g.mw)], mw...),
		path: joinPath(g.path, path),
		s:    g.s,
	}
}

func joinPath(p1, p2 string) string {
	if p2 == "" {
		return p1
	}

	if p1 != "" && p1[0] != '/' {
		p1 = "/" + p1
	}

	if p2 != "" && p2[0] != '/' {
		p2 = "/" + p2
	}
	return strings.ReplaceAll(p1+p2, "//", "/")
}

type groupHandlerChain struct {
	g  *Group
	hc []Handler
}

func (ghc *groupHandlerChain) Serve(rw http.ResponseWriter, req *http.Request, p router.Params) {
	var (
		ctx = getCtx(rw, req, p, ghc.g.s)

		mwIdx, hIdx int

		catchPanic func()
	)
	defer putCtx(ctx)

	if ph := ghc.g.s.PanicHandler; ph != nil {
		catchPanic = func() {
			if v := recover(); v != nil {
				fr := oerrs.Caller(2)
				ghc.g.s.PanicHandler(ctx, v, fr)
			}
		}
	}
	ctx.nextMW = func() {
		if catchPanic != nil {
			defer catchPanic()
		}
		for mwIdx < len(ghc.g.mw) && !ctx.done {
			h := ghc.g.mw[mwIdx]
			mwIdx++
			if r := h(ctx); r != nil {
				if r != Break {
					_ = r.WriteToCtx(ctx)
				} else {
					ctx.next = nil
				}
				break
			}
		}
		ctx.nextMW = nil
	}

	ctx.next = func() {
		if catchPanic != nil {
			defer catchPanic()
		}
		for hIdx < len(ghc.hc) && !ctx.done {
			h := ghc.hc[hIdx]
			hIdx++
			if r := h(ctx); r != nil {
				if r != Break {
					_ = r.WriteToCtx(ctx)
				}
				break
			}
		}
		ctx.next = nil
	}

	ctx.Next()
}
