package router

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type OnRequestDone = func(ctx context.Context, group, method, uri string, duration time.Duration)

// Options passed to the router
type Options struct {
	OnRequestDone

	APIInfo *SwaggerInfo

	NoAutoCleanURL           bool // don't automatically clean URLs, not recommended
	NoDefaultPanicHandler    bool // don't use the default panic handler
	NoPanicOnInvalidAddRoute bool // don't panic on invalid routes, return an error instead
	CatchPanics              bool // don't catch panics
	NoAutoHeadToGet          bool // disable automatically handling HEAD requests
	ProfileLabels            bool
	AutoGenerateSwagger      bool
}

const (
	// tooManyStars is returned if there are multiple *params in the path
	tooManyStars = "too many stars"
	// starNotLast is returned if *param is not the last part of the path.
	starNotLast = "star param must be the last part of the path"
)

type Route struct {
	r        *Router
	m        string
	g        string
	fp       string
	h        Handler
	parts    []nodePart
	disabled atomic.Bool
}

func (n *Route) hasStar() bool {
	return len(n.parts) > 0 && n.parts[len(n.parts)-1].Type() == '*'
}

func (n *Route) paramLen() (out int) {
	for _, p := range n.parts {
		if t := p.Type(); t == ':' || t == '*' {
			out++
		}
	}
	return
}

func (n *Route) getPartName(i int) (np string, pi int) {
	for i < len(n.parts) {
		np = string(n.parts[i])
		pi++
		if t := np[0]; t != '*' && t != ':' {
			continue
		}
		np = np[1:]
		break
	}
	return
}

func (r *Route) Path() string {
	return r.fp
}

func (r *Route) Group() string {
	return r.g
}

func (r *Route) Handler() Handler {
	return r.h
}

func (n *Route) WithDoc(desc string, genParams bool) *SwaggerRoute {
	sr := &SwaggerRoute{
		Description: desc,
	}

	if genParams {
		for _, p := range n.parts {
			if p[0] == ':' || p[0] == '*' {
				sr = sr.WithParam(p.Name(), p.String()+" is required", "path", "string", true, nil)
			}
		}
	}
	n.r.addRouteInfo(n.m, n.fp, sr)
	return sr
}

type routeMap map[string][]*Route

func (rm routeMap) get(path string) []*Route {
	return rm[path]
}

func (rm routeMap) append(path string, n *Route) {
	rm[path] = append(rm[path], n)
}

// Router is an efficient routing library
type Router struct {
	methods [9]routeMap
	swagger Swagger

	pp sync.Pool

	NotFoundHandler         Handler
	MethodNotAllowedHandler Handler
	PanicHandler            PanicHandler

	opts      Options
	maxParams int
}

// New returns a new Router
func New(opts *Options) *Router {
	var r Router

	if opts != nil {
		r.opts = *opts
	}

	r.pp.New = func() any {
		return &paramsWrapper{make(Params, 0, r.maxParams)}
	}

	if !r.opts.NoDefaultPanicHandler {
		r.PanicHandler = DefaultPanicHandler
	}

	r.swagger.OpenAPI = "3.0.3"
	r.swagger.Info = r.opts.APIInfo
	return &r
}

func (r *Router) GetRoutes() [][3]string {
	rms := r.getAllMaps()
	routes := make([][3]string, 0, len(rms))
	for method, rm := range rms {
		for p, ns := range rm {
			base := p
			for _, n := range ns {
				route := base
				for _, np := range n.parts {
					route += "/" + string(np)
				}
				routes = append(routes, [3]string{n.g, method, route})
			}
		}
	}
	return routes
}

// AddRoute adds a Handler to the specific method and route.
// Calling AddRoute after starting the http server is racy and not supported.
func (r *Router) AddRoute(group, method, route string, h Handler) *Route {
	return r.AddRouteWithDesc(group, method, route, h, "")
}

// AddRoute adds a Handler to the specific method and route.
// Calling AddRoute after starting the http server is racy and not supported.
func (r *Router) AddRouteWithDesc(group, method, route string, h Handler, desc string) *Route {
	p, rest, num, stars := splitPathToParts(route)
	if stars > 1 {
		panic(tooManyStars)
	}

	if stars == 1 && rest[len(rest)-1].Type() != '*' {
		panic(starNotLast)
	}

	if n := len(p) - 1; len(p) > 1 && p[n] == '/' {
		p = p[:n]
	}

	m := r.getMap(method, true)
	n := &Route{r: r, fp: route, g: group, m: method, h: h, parts: rest}
	m.append(p, n)

	if num > r.maxParams {
		r.maxParams = num
	}

	if desc != "" && r.opts.AutoGenerateSwagger {
		n.WithDoc(desc, r.opts.AutoGenerateSwagger)
	}
	return n
}

// Match matches a method and path to a handler.
// if METHOD == HEAD and there isn't a specific handler for it, it returns the GET handler for the path.
func (r *Router) Match(method, path string) (rn *Route, params Params) {
	rr, p := r.match(method, path)

	if rr == nil && method == http.MethodHead && !r.opts.NoAutoHeadToGet {
		rr, p = r.match(http.MethodGet, path)
	}

	return rr, p.Params()
}

func (r *Router) DisableRoute(method, path string, disabled bool) bool {
	if rr, _ := r.Match(method, path); rr != nil {
		rr.disabled.Store(disabled)
		return true
	}
	return false
}

func (r *Router) match(method, path string) (rn *Route, params *paramsWrapper) {
	m := r.getMap(method, false)
	var (
		nn   []*Route
		nsep int
	)

	if !revSplitPathFn(path, '/', func(p string, pidx, idx int) bool {
		if nn = m.get(path[:idx]); nn != nil {
			path, nsep = path[idx:], pidx
			return true
		}

		return false
	}) {
		if nn = m.get("/"); nn != nil {
			nsep = strings.Count(path, "/")
		} else {
			return
		}
	}

	for _, n := range nn {
		if len(n.parts) == nsep || n.hasStar() {
			rn = n
			break
		}
	}

	if rn == nil || len(rn.parts) == 0 {
		return
	}

	params = r.getParams()
	splitPathFn(path, '/', func(p string, pidx, idx int) bool {
		np := rn.parts[pidx]
		switch np.Type() {
		case ':':
			params.p = append(params.p, Param{np.Name(), p[1:]})
		case '*':
			params.p = append(params.p, Param{np.Name(), path[1:]})
			return true
		}
		return false
	})

	return
}

func (r *Router) getAllMaps() map[string]routeMap {
	out := make(map[string]routeMap)
	for i, rm := range &r.methods {
		switch i {
		case 0:
			out[http.MethodGet] = rm
		case 1:
			out[http.MethodHead] = rm
		case 2:
			out[http.MethodPost] = rm
		case 3:
			out[http.MethodPut] = rm
		case 4:
			out[http.MethodPatch] = rm
		case 5:
			out[http.MethodDelete] = rm
		case 6:
			out[http.MethodConnect] = rm
		case 7:
			out[http.MethodOptions] = rm
		case 8:
			out[http.MethodTrace] = rm
		}
	}
	return out
}

func (r *Router) getMap(method string, create bool) routeMap {
	var rm *routeMap
	switch method {
	case http.MethodGet:
		rm = &r.methods[0]
	case http.MethodHead:
		rm = &r.methods[1]
	case http.MethodPost:
		rm = &r.methods[2]
	case http.MethodPut:
		rm = &r.methods[3]
	case http.MethodPatch:
		rm = &r.methods[4]
	case http.MethodDelete:
		rm = &r.methods[5]
	case http.MethodConnect:
		rm = &r.methods[6]
	case http.MethodOptions:
		rm = &r.methods[7]
	case http.MethodTrace:
		rm = &r.methods[8]
	default:
		return nil
	}
	if create && *rm == nil {
		*rm = routeMap{}
	}

	return *rm
}

func (r *Router) getParams() *paramsWrapper {
	// this should never ever panic, if it does then there's something extremely wrong and *it should* panic
	return r.pp.Get().(*paramsWrapper)
}

func (r *Router) putParams(p *paramsWrapper) {
	if p == nil || cap(p.p) != r.maxParams {
		return
	}
	p.p = p.p[:0]
	r.pp.Put(p)
}
