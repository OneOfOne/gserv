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

// LogRequests is a request logger middleware.
// If logJSONRequests is true, it'll attempt to parse the incoming request's body and output it to the log.
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
				io.Copy(&buf, req.Body)
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

// SecureCookie is a middleware to enable SecureCookies.
// For more details check `go doc securecookie.New`
func SecureCookie(hashKey, blockKey []byte) Handler {
	return func(ctx *Context) Response {
		ctx.Set(secureCookieKey, securecookie.New(hashKey, blockKey))
		return nil
	}
}

// GetSecureCookie returns the *securecookie.SecureCookie associated with the Context, or nil.
func GetSecureCookie(ctx *Context) *securecookie.SecureCookie {
	sc, ok := ctx.Get(secureCookieKey).(*securecookie.SecureCookie)
	if ok {
		return sc
	}
	return nil
}
