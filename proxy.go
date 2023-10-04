package gserv

import (
	"net/http"
	"net/http/httputil"
	"strings"
)

var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te", // canonicalized version of "TE"
	"Trailers",
	"Transfer-Encoding",
	"Accept-Encoding",
	"Upgrade",
}

func ProxyHandler(host string, pathFn func(string) string) Handler {
	rp := &httputil.ReverseProxy{}

	scheme := "http"
	if strings.HasPrefix(host, "http://") {
		host = host[7:]
	} else if strings.HasPrefix(host, "https://") {
		scheme = "https"
		host = host[8:]
	}

	rp.Director = func(req *http.Request) {
		req.URL.Scheme = scheme
		req.URL.Host = host
		req.Host = ""

		if pathFn != nil {
			req.URL.Path = pathFn(req.URL.Path)
		}

		h := req.Header
		if _, ok := h["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to default value
			req.Header.Set("User-Agent", "")
		}

		for _, hh := range hopHeaders {
			h.Del(hh)
		}

		h.Set("X-Forwarded-For", req.RemoteAddr)
	}

	rp.ModifyResponse = func(r *http.Response) error {
		return nil
	}

	return func(ctx *Context) Response {
		rp.ServeHTTP(ctx, ctx.Req)
		return nil
	}
}
