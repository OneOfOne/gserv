package gserv

import (
	"net/http"
	"testing"
)

func TestProxy(t *testing.T) {
	s := newServerAndWait(t, "localhost:0")
	defer s.Shutdown(0)

	worked := false
	s.GET("/api/*fn", func(ctx *Context) Response {
		t.Log("got", ctx.Path())
		worked = true
		return RespOK
	})

	s2 := newServerAndWait(t, "localhost:0")
	defer s2.Shutdown(0)

	s2.GET("/*fn", ProxyHandler(s.Addrs()[0], func(_ *http.Request, s string) string {
		return "/api/" + s
	}))

	http.Get("http://" + s2.Addrs()[0] + "/x")
	http.Get("http://" + s2.Addrs()[0] + "/y")
	http.Get("http://" + s2.Addrs()[0] + "/z/x")

	if !worked {
		t.Fatal("proxy didn't work")
	}
}
