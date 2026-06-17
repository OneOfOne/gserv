package gserv

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/securecookie"
)

func TestSecureCookie(t *testing.T) {
	srv := newServerAndWait(t, "")
	defer srv.Shutdown(0)

	srv.Use(SecureCookie(bytes.Repeat([]byte("1"), 32), securecookie.GenerateRandomKey(32)))

	JSONGet(srv, "/", func(ctx *Context) (any, error) {
		_ = ctx.SetCookie("cooookie", M{"stuff": "and things"}, "", false, time.Hour)
		return nil, nil
	}, true)

	JSONGet(srv, "/cookie", func(ctx *Context) (M, error) {
		var m M
		_ = ctx.GetCookieValue("cooookie", &m)
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

func TestRateLimiter_DownstreamHeadersOnBlockedResponse(t *testing.T) {
	srv := newServerAndWait(t, "")
	defer srv.Shutdown(0)

	srv.Use(RateLimiter(context.Background(), nil, 1, 100, 1, true))
	JSONGet(srv, "/rl", func(ctx *Context) (string, error) {
		return "ok", nil
	}, true)

	url := "http://" + srv.Addrs()[0] + "/rl"

	// First request — should succeed with non-zero remaining and no Retry-After
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	remaining := resp.Header.Get("X-Rate-Limit-Remaining")
	if remaining == "" {
		t.Fatal("expected X-Rate-Limit-Remaining on allowed response")
	}
	if remaining == "0s" {
		t.Fatalf("expected non-zero remaining, got %q", remaining)
	}

	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter != "" {
		t.Fatalf("unexpected Retry-After on allowed response: %q", retryAfter)
	}

	// Second request — should be blocked with 429
	resp, err = http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, resp.StatusCode)
	}

	retryAfterVal := resp.Header.Get("Retry-After")
	if retryAfterVal == "" {
		t.Fatal("expected Retry-After on blocked response")
	}
	if _, err := strconv.Atoi(retryAfterVal); err != nil {
		t.Fatalf("invalid Retry-After value %q: %v", retryAfterVal, err)
	}

	resetHeader := resp.Header.Get("X-Rate-Limit-Reset")
	if resetHeader == "" {
		t.Fatal("expected X-Rate-Limit-Reset on blocked response")
	}
	if resetHeader != retryAfterVal {
		t.Fatalf("X-Rate-Limit-Reset (%q) should equal Retry-After (%q)", resetHeader, retryAfterVal)
	}

	remaining = resp.Header.Get("X-Rate-Limit-Remaining")
	if remaining == "" {
		t.Fatal("expected X-Rate-Limit-Remaining on blocked response")
	}
	// With limit=1 per second and the second request blocked, remaining should be 0s for seconds
	if !strings.Contains(remaining, "0s") {
		t.Fatalf("expected remaining to contain '0s' when blocked by per-second, got %q", remaining)
	}
}

func TestRateLimiter_MiddlewareDecreasingRemaining(t *testing.T) {
	srv := newServerAndWait(t, "")
	defer srv.Shutdown(0)

	srv.Use(RateLimiter(context.Background(), nil, 5, 100, 100, true))
	JSONGet(srv, "/rl", func(ctx *Context) (string, error) {
		return "ok", nil
	}, true)

	base := "http://" + srv.Addrs()[0] + "/rl"
	var allRemaining []string

	// Make 6 sequential requests with unique query params to avoid caching
	for i := 0; i < 6; i++ {
		resp, err := http.Get(base + "?n=" + strconv.Itoa(i))
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		remaining := resp.Header.Get("X-Rate-Limit-Remaining")
		allRemaining = append(allRemaining, remaining)

		if i < 5 {
			// First 5 should succeed
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("request %d: expected status %d, got %d (body: %s)", i+1, http.StatusOK, resp.StatusCode, string(body))
			}
		} else {
			// 6th should be blocked
			if resp.StatusCode != http.StatusTooManyRequests {
				t.Fatalf("request %d: expected status %d, got %d (body: %s)", i+1, http.StatusTooManyRequests, resp.StatusCode, string(body))
			}
		}
	}

	// Verify per-second remaining decreases monotonically from 4 to 0 across the 5 allowed requests.
	// We parse the first field (per-second) from each response to avoid brittle exact-match on minute/hour
	// which can drift if requests span second boundaries.
	var secRemaining []int
	for _, r := range allRemaining {
		parts := strings.Split(r, ", ")
		if len(parts) < 1 {
			t.Fatalf("unexpected remaining format %q", r)
		}
		s := strings.TrimSuffix(parts[0], "s")
		v, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("request %d: failed to parse per-second remaining from %q: %v", len(secRemaining)+1, parts[0], err)
		}
		secRemaining = append(secRemaining, v)
	}

	for i := 1; i < 5; i++ { // only verify monotonic decrease among allowed requests
		if secRemaining[i] >= secRemaining[i-1] {
			t.Fatalf("request %d: per-second remaining did not decrease (prev=%d, curr=%d)", i+1, secRemaining[i-1], secRemaining[i])
		}
	}

	if secRemaining[0] != 4 || secRemaining[4] != 0 {
		t.Fatalf("expected first per-second remaining=4 and fifth=0, got %v", secRemaining)
	}
}

func TestRateLimiter_MiddlewareNilLimitKeyDefaultsToClientIP(t *testing.T) {
	srv := newServerAndWait(t, "")
	defer srv.Shutdown(0)

	srv.Use(RateLimiter(context.Background(), nil, 1, 100, 1, false))
	JSONGet(srv, "/rl", func(ctx *Context) (string, error) {
		return "ok", nil
	}, true)

	base := "http://" + srv.Addrs()[0] + "/rl"

	var resp1, resp2 *http.Response
	var err1, err2 error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		resp1, err1 = http.Get(base)
		if resp1 != nil {
			_ = resp1.Body.Close()
		}
	}()

	go func() {
		// Small delay to increase chance of concurrency overlap
		time.Sleep(10 * time.Millisecond)
		defer wg.Done()
		resp2, err2 = http.Get(base)
		if resp2 != nil {
			_ = resp2.Body.Close()
		}
	}()

	wg.Wait()

	if err1 != nil {
		t.Fatalf("first request failed: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("second request failed: %v", err2)
	}

	// Both requests share the same client IP (localhost), so only one should succeed.
	okCount := 0
	if resp1.StatusCode == http.StatusOK {
		okCount++
	}
	if resp2.StatusCode == http.StatusOK {
		okCount++
	}
	if okCount != 1 {
		t.Fatalf("expected exactly one request to succeed (same IP), got %d: resp1=%d, resp2=%d", okCount, resp1.StatusCode, resp2.StatusCode)
	}
}
