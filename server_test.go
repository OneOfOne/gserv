package gserv

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"go.oneofone.dev/gserv/router"
	"go.oneofone.dev/otk"
)

func init() {
	log.SetFlags(0)
}

var testData = []struct {
	path string
	*JSONResponse
}{
	{"/ping", NewJSONResponse("pong")},
	{"/ping/world", NewJSONResponse("pong:world")},
	{"/random", NewJSONErrorResponse(404)},
	{"/panic", NewJSONErrorResponse(http.StatusInternalServerError, "internal server error")},
	{"/panic2", NewJSONErrorResponse(http.StatusInternalServerError, "internal server error")},
	{"/mw/sub/id", NewJSONResponse("data:/mw/sub/:id")},
	{"/mw/sub/disabled/id", NewJSONErrorResponse(404)},
}

func newServerAndWait(t *testing.T, addr string) *Server {
	var (
		s     *Server
		timer = time.After(time.Second)
	)
	if testing.Verbose() {
		s = New()
	} else {
		s = New(SetErrLogger(nil)) // don't need the spam with panics for the /panic handler
	}
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	go s.Run(context.Background(), addr)
	for {
		select {
		case <-timer:
			t.Errorf("still no address after 1 second")
		default:
		}
		addrs := s.Addrs()
		if len(addrs) == 0 {
			time.Sleep(time.Millisecond)
			continue
		}
		if strings.HasPrefix(addrs[0], ":0") {
			t.Errorf("unexpected addr: %v", addrs[0])
		}
		return s
	}
}

func TestErrors(t *testing.T) {
	j, _ := json.Marshal(RespBadRequest)
	if !strings.Contains(string(j), `"errors":[{"message":"Bad Request"}]`) {
		t.Fatalf("bad marshal: %s", j)
	}
}

func cmpData(a, b any) bool {
	av, bv := a, b
	if ab, ok := a.(*otk.Buffer); ok {
		av = nil
		json.Unmarshal(ab.Bytes(), &av)
	}
	if bb, ok := b.(*otk.Buffer); ok {
		bv = nil
		json.Unmarshal(bb.Bytes(), &bv)
	}
	return av == bv
}

var pong = "pong"

func panicTyped(ctx *Context) (any, error) {
	panic("well... poo")
}

func TestServer(t *testing.T) {
	var srv *Server

	if testing.Verbose() {
		srv = New(SetCatchPanics(true))
	} else {
		srv = New(SetCatchPanics(true), SetErrLogger(nil)) // don't need the spam with panics for the /panic handler
	}

	// srv.PanicHandler = func(ctx *Context, v any) {
	// 	r := NewJSONErrorResponse(500, fmt.Sprintf("PANIC (%T): %v\n", v, v))
	// 	r.WriteToCtx(ctx)
	// }

	JSONGet(srv, "/ping", func(ctx *Context) (string, error) {
		// ctx.Logf("you wut m8: %v", ctx.ReqHeader())
		return "pong", nil
	}, true)

	srv.GET("/panic", func(ctx *Context) Response {
		panic("well... poo")
	})

	JSONGet(srv, "/panic2", panicTyped, true)
	srv.AllowCORS("/cors", "GET")

	JSONGet(srv, "/ping/:id", func(ctx *Context) (string, error) {
		return "pong:" + ctx.Params.Get("id"), nil
	}, true)

	type PingReq struct {
		Ping string `json:"ping"`
	}

	JSONPost(srv, "/ping/:id", func(ctx *Context, req *PingReq) (string, error) {
		r := router.RouteFromRequest(ctx.Req)
		if r == nil {
			t.Fatal("couldn't get route from request")
		}
		if rp := r.Path(); rp != "/ping/:id" {
			t.Fatalf("expected /ping/:id, got %s", rp)
		}
		return "pong:" + ctx.Params.Get("id") + ":" + req.Ping, nil
	}, true)

	srv.Static("/s/", "./", false)
	srv.Static("/s-std/", "./", true)

	srv.StaticFile("/README.md", "./router/README.md")

	sg := srv.SubGroup("groupName", "/mw", func(ctx *Context) Response {
		r := ctx.Route()
		if r == nil {
			t.Fatal("couldn't get route from request")
		}
		ctx.Set("data", r.Path())
		return nil
	})
	sg.GET("/sub/:id", func(ctx *Context) Response {
		v, _ := ctx.Get("data").(string)
		return NewJSONResponse("data:" + v)
	})

	sg.GET("/sub/disabled/:id", func(ctx *Context) Response {
		v, _ := ctx.Get("data").(string)
		return NewJSONResponse("data:" + v)
	})

	if !srv.DisableRoute("GET", "/mw/sub/disabled/:id", true) {
		t.Error("expected DisableRoute to return true")
	}

	//	srv.Use(LogRequests(true))

	ts := httptest.NewServer(srv)
	defer ts.Close()

	for _, td := range testData {
		t.Run("EP="+td.path, func(t *testing.T) {
			var (
				res, err = http.Get(ts.URL + td.path)
				resp     JSONResponse
			)
			if err != nil {
				t.Error(td.path, err)
			}
			b, _ := io.ReadAll(res.Body)
			err = json.NewDecoder(bytes.NewReader(b)).Decode(&resp)
			res.Body.Close()
			if err != nil {
				t.Error(td.path, err)
			}

			if resp.Code != td.Code || !cmpData(resp.Data, td.Data) {
				t.Errorf("expected (%s) (%v) %#+v, got (%v) %#+v", td.path, td.Code, td.Data, resp.Code, resp.Data)
			}

			if len(resp.Errors) > 0 {
				if len(resp.Errors) != len(td.Errors) {
					t.Fatalf("expected (%s) %+v, got %+v", td.path, td.JSONResponse, resp)
				}

				for i := range resp.Errors {
					if re, te := resp.Errors[i], td.Errors[i]; !strings.HasPrefix(re.Error(), te.Error()) {
						t.Fatalf("expected %q, got %q", te, re)
					}
				}
			}
		})
	}

	t.Run("Static", func(t *testing.T) {
		readme, _ := os.ReadFile("./router/README.md")
		res, err := http.Get(ts.URL + "/s/router/README.md")
		if err != nil {
			t.Error(err)
		}

		b, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Error(err)
		}

		if !bytes.Equal(readme, b) {
			t.Error("files not equal", string(b))
		}

		res, err = http.Get(ts.URL + "/s-std/router/README.md")

		if err != nil {
			t.Error(err)
		}

		b, err = io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Error(err)
		}

		if !bytes.Equal(readme, b) {
			t.Error("files not equal")
		}

		res, err = http.Get(ts.URL + "/s-std")

		if err != nil {
			t.Error(err)
		}

		b, err = io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Error(err)
		}

		if !bytes.Contains(b, []byte(`<a href=".git/">.git/</a>`)) {
			t.Error("unexpected output", string(b))
		}

		res, err = http.Get(ts.URL + "/s")

		if err != nil {
			t.Error(err)
		}

		b, err = io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Error(err)
		}

		if !bytes.Contains(b, []byte(`404 page not found`)) {
			t.Error("unexpected output", string(b))
		}

		res, err = http.Get(ts.URL + "/README.md")

		if err != nil {
			t.Error(err)
		}

		b, err = io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Error(err)
		}

		if !bytes.Equal(readme, b) {
			t.Error("files not equal", string(b))
		}
	})

	t.Run("ReadResp", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/ping")
		if err != nil {
			t.Error(err)
		}

		var s string
		r, err := ReadJSONResponse(res.Body, &s)
		if err != nil {
			t.Error(err)
		}
		if s != "pong" {
			t.Errorf("expected pong, got %+v", r)
		}
	})

	t.Run("CORS", func(t *testing.T) {
		var (
			client http.Client
			req, _ = http.NewRequest(http.MethodOptions, ts.URL+"/cors", nil)
		)
		req.Header.Add("Origin", "http://localhost")
		resp, _ := client.Do(req)
		resp.Body.Close()
		if resp.Header.Get("Access-Control-Allow-Methods") != "GET" {
			t.Errorf("unexpected headers: %+v", resp.Header)
		}
	})

	t.Run("POST", func(t *testing.T) {
		resp, err := http.Post(ts.URL+"/ping/hello", MimeJSON, strings.NewReader(`{"ping": "world"}`))
		if err != nil {
			t.Error(err)
		}
		defer resp.Body.Close()
		var s string

		if _, err = ReadJSONResponse(resp.Body, &s); err != nil {
			t.Error(err)
		}
		if s != "pong:hello:world" {
			t.Errorf("expected pong:hello:world, got %#+v", s)
		}
	})
}

func TestListenZero(t *testing.T) {
	s := newServerAndWait(t, "")
	defer s.Shutdown(0)
}
