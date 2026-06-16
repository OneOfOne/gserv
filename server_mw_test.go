package gserv

import (
	"bytes"
	"context"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/securecookie"
)

func TestSecureCookie(t *testing.T) {
	srv := newServerAndWait(t, "")
	defer srv.Shutdown(0)

	srv.Use(SecureCookie(bytes.Repeat([]byte("1"), 32), securecookie.GenerateRandomKey(32)))

	JSONGet(srv, "/", func(ctx *Context) (any, error) {
		ctx.SetCookie("cooookie", M{"stuff": "and things"}, "", false, time.Hour)
		return nil, nil
	}, true)

	JSONGet(srv, "/cookie", func(ctx *Context) (M, error) {
		var m M
		ctx.GetCookieValue("cooookie", &m)
		return m, nil
	}, true)

	addr := srv.Addrs()[0]

	var cli http.Client
	cli.Jar, _ = cookiejar.New(nil)

	resp, err := cli.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	cs := resp.Cookies()

	if len(cs) != 1 {
		t.Fatal("couldn't find the cookie :(")
	}

	resp, err = cli.Get("http://" + addr + "/cookie")
	if err != nil {
		t.Fatal(err)
	}

	var respValue M
	if _, err = ReadJSONResponse(resp.Body, &respValue); err != nil {
		t.Fatal(err)
	}

	if respValue["stuff"] != "and things" {
		t.Fatalf("unexpected response: %#+v", respValue)
	}
}

func TestRateLimiter_WithHeaders(t *testing.T) {
	srv := newServerAndWait(t, "")
	defer srv.Shutdown(0)

	srv.Use(RateLimiter(context.Background(), nil, 100, 100, 1, true))
	JSONGet(srv, "/rl", func(ctx *Context) (string, error) {
		return "ok", nil
	}, true)

	url := "http://" + srv.Addrs()[0] + "/rl"

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got := resp.Header.Get("X-Rate-Limit-Limit"); got != "100s, 100m, 1h" {
		t.Fatalf("unexpected X-Rate-Limit-Limit: %q", got)
	}

	if got := resp.Header.Get("X-Rate-Limit-Remaining"); got == "" {
		t.Fatal("expected X-Rate-Limit-Remaining on allowed response")
	}

	if got := resp.Header.Get("Retry-After"); got != "" {
		t.Fatalf("unexpected Retry-After on allowed response: %q", got)
	}

	resp, err = http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, resp.StatusCode)
	}

	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("expected Retry-After on blocked response")
	}

	if _, err := strconv.Atoi(retryAfter); err != nil {
		t.Fatalf("invalid Retry-After value %q: %v", retryAfter, err)
	}

	if got := resp.Header.Get("X-Rate-Limit-Reset"); got != retryAfter {
		t.Fatalf("expected X-Rate-Limit-Reset (%q) to equal Retry-After (%q)", got, retryAfter)
	}
}

func TestRateLimiter_WithoutHeaders(t *testing.T) {
	srv := newServerAndWait(t, "")
	defer srv.Shutdown(0)

	srv.Use(RateLimiter(context.Background(), nil, 100, 100, 1, false))
	JSONGet(srv, "/rl-no-headers", func(ctx *Context) (string, error) {
		return "ok", nil
	}, true)

	url := "http://" + srv.Addrs()[0] + "/rl-no-headers"

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got := resp.Header.Get("X-Rate-Limit-Limit"); got != "" {
		t.Fatalf("did not expect X-Rate-Limit-Limit, got %q", got)
	}

	resp, err = http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, resp.StatusCode)
	}

	if got := resp.Header.Get("Retry-After"); got != "" {
		t.Fatalf("did not expect Retry-After, got %q", got)
	}
}

func TestRateLimiter_CustomLimitKeyIsolation(t *testing.T) {
	srv := newServerAndWait(t, "")
	defer srv.Shutdown(0)

	srv.Use(RateLimiter(context.Background(), func(ctx *Context) string {
		return ctx.Req.URL.Query().Get("k")
	}, 100, 100, 1, false))

	JSONGet(srv, "/rl-key", func(ctx *Context) (string, error) {
		return "ok", nil
	}, true)

	base := "http://" + srv.Addrs()[0] + "/rl-key"

	resp, err := http.Get(base + "?k=a")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected first request for key a to pass, got %d", resp.StatusCode)
	}

	resp, err = http.Get(base + "?k=b")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected first request for key b to pass, got %d", resp.StatusCode)
	}

	resp, err = http.Get(base + "?k=a")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected second request for key a to be blocked, got %d", resp.StatusCode)
	}
}
