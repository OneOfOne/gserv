package gserv

import (
	"net/http"
	"testing"
)

// Using .Next in a middleware didn't execute other middlewars
func TestIssue12(t *testing.T) {
	s := newServerAndWait(t, "localhost:0")
	defer s.Shutdown(0)

	s.Use(LogRequests(true))
	g := s.SubGroup("", "", func(ctx *Context) error {
		ctx.Set("mw", true)
		return nil
	})

	JSONGet(g, "/ping", func(ctx *Context) (*JSONResponse, error) {
		if v, ok := ctx.Get("mw").(bool); !ok || !v {
			return RespNotFound.(*JSONResponse), nil
		}
		return NewJSONResponse(nil), nil
	}, false)

	resp, err := http.Get("http://" + s.Addrs()[0] + "/ping")
	if err != nil {
		t.Error(err)
		return
	}
	if resp.StatusCode != 200 {
		t.Error("couldn't get the ctx value")
	}
}
