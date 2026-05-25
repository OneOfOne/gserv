package gserv

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/securecookie"
	"go.oneofone.dev/gserv/internal"
)

var reqID uint64

// LogRequests returns a middleware that logs each request with its method, path, status code, duration, and client IP.
// If logJSONRequests is true, it also attempts to parse and log the incoming request body for JSON content types.
func LogRequests(logJSONRequests bool) Handler {
	return func(ctx *Context) Response {
		var (
			req   = ctx.Req
			url   = req.URL
			start = time.Now()
			id    = atomic.AddUint64(&reqID, 1)
			extra string
		)

		if logJSONRequests {
			switch m := req.Method; m {
			case http.MethodPost, http.MethodPut, http.MethodPatch:
				var buf bytes.Buffer
				_, _ = io.Copy(&buf, req.Body)
				req.Body.Close()
				req.Body = io.NopCloser(&buf)
				j, _ := internal.Marshal(req.Header)
				if ln := buf.Len(); ln > 0 {
					switch buf.Bytes()[0] {
					case '[', '{', 'n': // [], {} and nullable
						extra = fmt.Sprintf("\n\tHeaders: %s\n\tRequest (%d): %s", j, ln, buf.String())
					default:
						extra = fmt.Sprintf("\n\tHeaders: %s\n\tRequest (%d): <binary>", j, buf.Len())
					}
				}
			}
		}

		ctx.NextMiddleware()
		ctx.Next()

		ct := req.Header.Get("Content-Type")

		switch ct {
		case MimeJSON:
			ct = "[JSON] "
		case MimeMsgPack:
			ct = "[MSGP] "
		case MimeEvent:
			ct = "[SSE] "
		case "":
		default:
			ct = "[" + ct + "] "
		}

		ctx.LogSkipf(1, "[reqID:%05d] [%s] [%s] %s[%d] %s %s [%s]%s",
			id, ctx.ClientIP(), req.UserAgent(), ct, ctx.Status(), req.Method, url.Path, time.Since(start), extra)
		return nil
	}
}

const secureCookieKey = ":SC:"

// SecureCookie is a middleware that enables SecureCookies for the context.
// For more details check `go doc securecookie.New`
func SecureCookie(hashKey, blockKey []byte) Handler {
	return func(ctx *Context) Response {
		ctx.Set(secureCookieKey, securecookie.New(hashKey, blockKey))
		return nil
	}
}

// GetSecureCookie retrieves the SecureCookie from the context, or nil if not set.
func GetSecureCookie(ctx *Context) *securecookie.SecureCookie {
	sc, ok := ctx.Get(secureCookieKey).(*securecookie.SecureCookie)
	if ok {
		return sc
	}
	return nil
}
