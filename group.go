package gserv

import (
	"net/http"
	"strings"

	"go.oneofone.dev/gserv/router"
	"go.oneofone.dev/oerrs"
)

type (
	Route = *router.Route
)

var DefaultCodec Codec = &JSONCodec{}

// Handler is the default server Handler
// In a handler chain, returning a non-nil breaks the chain.
type Handler = func(ctx *Context) Response

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

// Routes returns the current routes set.
// Each route is returned in the order of group name, method, path.
func (g *Group) Routes() [][3]string {
	return g.s.r.GetRoutes()
}

// AddRoute adds a handler (or more) to the specific method and path
// it is NOT safe to call this once you call one of the run functions
func (g *Group) AddRoute(method, path string, handlers ...Handler) Route {
	ghc := groupHandlerChain{
		hc: handlers,
		g:  g,
	}
	p := joinPath(g.path, path)
	return g.s.r.AddRoute(g.nm, method, p, ghc.Serve)
}

// GET is an alias for AddRoute("GET", path, handlers...).
func (g *Group) GET(path string, handlers ...Handler) Route {
	return g.AddRoute(http.MethodGet, path, handlers...)
}

// PUT is an alias for AddRoute("PUT", path, handlers...).
func (g *Group) PUT(path string, handlers ...Handler) Route {
	return g.AddRoute(http.MethodPut, path, handlers...)
}

// POST is an alias for AddRoute("POST", path, handlers...).
func (g *Group) POST(path string, handlers ...Handler) Route {
	return g.AddRoute(http.MethodPost, path, handlers...)
}

// DELETE is an alias for AddRoute("DELETE", path, handlers...).
func (g *Group) DELETE(path string, handlers ...Handler) Route {
	return g.AddRoute(http.MethodDelete, path, handlers...)
}

// OPTIONS is an alias for AddRoute("OPTIONS", path, handlers...).
func (g *Group) OPTIONS(path string, handlers ...Handler) Route {
	return g.AddRoute(http.MethodOptions, path, handlers...)
}

func (g *Group) DisableRoute(method, path string, disabled bool) bool {
	return g.s.r.DisableRoute(method, joinPath(g.path, path), disabled)
}

func (g *Group) Static(path, localPath string, allowListing bool) Route {
	path = strings.TrimSuffix(path, "/")

	return g.AddRoute(http.MethodGet, joinPath(path, "*fp"), StaticDirStd(path, localPath, allowListing))
}

func (g *Group) StaticFile(path, localPath string) Route {
	return g.AddRoute(http.MethodGet, path, func(ctx *Context) Response {
		ctx.File(localPath)
		return nil
	})
}

// SubGroup returns a sub-handler group based on the current group's middleware
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
					r.WriteToCtx(ctx)
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
					r.WriteToCtx(ctx)
				}
				break
			}
		}
		ctx.next = nil
	}

	ctx.Next()
}
