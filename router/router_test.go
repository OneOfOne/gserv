package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRouter(t *testing.T) {
	r := buildAPIRouter(t, false)
	r.opts.AutoGenerateSwagger = true
	for _, m := range restAPIRoutes {
		ep := m.url
		req, _ := http.NewRequest("GET", ep, nil)
		r.ServeHTTP(nil, req)
		req, _ = http.NewRequest("PATCH", ep, nil)
		r.ServeHTTP(nil, req)
		req, _ = http.NewRequest("PATCH", "../"+ep, nil)
		r.ServeHTTP(nil, req)
	}
	j, _ := json.MarshalIndent(r.swagger, "", "  ")
	t.Log("\n" + string(j))
}

func TestRouterStar(t *testing.T) {
	r := New(nil)
	fn := func(_ http.ResponseWriter, req *http.Request, p Params) {}
	r.AddRoute("", "GET", "/home", nil)
	r.AddRoute("", "GET", "/home/*path", fn)
	if rn, p := r.Match("GET", "/home"); rn == nil || len(p) != 0 {
		t.Fatalf("expected a match, got %v %v", rn, len(p))
	}
	if rn, p := r.Match("GET", "/home/file"); rn == nil || len(p) != 1 || p.Get("path") != "file" {
		t.Fatalf("expected a 1 match, got %v %v", rn, p)
	}
	if rn, p := r.Match("GET", "/home/file/file2/report.json"); rn == nil || len(p) != 1 || p.Get("path") != "file/file2/report.json" {
		t.Fatalf("expected a 1 match, got %v %v", rn, p)
	}
}

func BenchmarkRouter5Params(b *testing.B) {
	req, _ := http.NewRequest("GET", "/campaignReport/:id/:cid/:start-date/:end-date/:filename", nil)
	r := buildAPIRouter(b, false)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.ServeHTTP(nil, req)
		}
	})
}

func BenchmarkRouterStatic(b *testing.B) {
	req, _ := http.NewRequest("GET", "/dashboard", nil)
	r := buildAPIRouter(b, false)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.ServeHTTP(nil, req)
		}
	})
}

func buildAPIRouter(tb testing.TB, print bool) (r *Router) {
	tb.Helper()
	r = New(nil)
	r.PanicHandler = nil
	for _, m := range restAPIRoutes {
		ep := m.url
		cnt := strings.Count(ep, ":")
		fn := func(_ http.ResponseWriter, req *http.Request, p Params) {
			if ep != req.URL.EscapedPath() {
				tb.Fatalf("urls don't match, expected %s, got %s", ep, req.URL.EscapedPath())
			}
			if cnt != len(p) {
				tb.Fatalf("{%q: %q} expected %d params, got %d", ep, p, cnt, len(p))
			}
			if print {
				tb.Logf("[%s] %s %q", req.Method, ep, p)
			}
		}

		r.AddRoute("", "GET", ep, fn).WithDoc("this does stuff", true)

		r.AddRoute("", "PATCH", ep, fn)
	}
	r.NotFoundHandler = func(_ http.ResponseWriter, req *http.Request, _ Params) {
		tb.Fatalf("not found: %s", req.URL)
	}
	return r
}
