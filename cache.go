package gserv

import (
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.oneofone.dev/genh"
)

type cacheItem struct {
	value   Response
	headers http.Header
	created int64
}

type cacheMap = genh.LMap[string, *cacheItem]

func cleanCache(m *cacheMap, ttl int64) {
	for {
		now := time.Now().Unix()
		m.Update(func(m map[string]*cacheItem) {
			for k, it := range m {
				if now > it.created+ttl {
					delete(m, k)
				}
			}
		})
		time.Sleep(time.Second * time.Duration(ttl))
	}
}

// CacheHandler returns a caching middleware that caches responses based on an ETag.
// It checks the Cache-Control header for "no-cache" or "max-age=0" and bypasses the cache if found.
// The etag function generates a unique identifier per request, which is used as the cache key.
// If the ttlDuration is greater than 0, a background goroutine periodically cleans expired cache items.
// Cached responses must implement the CacheableResponse interface to be stored.
func CacheHandler(etag func(ctx *Context) string, ttlDuration time.Duration, handler Handler) Handler {
	c := cacheMap{}
	ttl := int64(ttlDuration.Seconds())
	if ttlDuration > 0 {
		go cleanCache(&c, ttl)
	}

	maxAge := "max-age=" + strconv.FormatInt(ttl, 10)

	return func(ctx *Context) Response {
		if ct := ctx.ReqHeader("Cache-Control"); strings.Contains(ct, "no-cache") || strings.Contains(ct, "max-age=0") {
			return handler(ctx)
		}

		tag := etag(ctx)
		if tag == "-" || tag == "" {
			return handler(ctx)
		}

		if _, ok := ctx.ResponseWriter.(*gzipRW); !ok {
			// less likely to trigger
			tag += ":0"
		}

		it := c.MustGet(tag, func() *cacheItem {
			resp := handler(ctx)
			if cr, ok := resp.(CacheableResponse); ok {
				resp = cr.Cached()
			}
			return &cacheItem{
				created: time.Now().Unix(),
				headers: ctx.Header(),
				value:   resp,
			}
		})

		h := ctx.Header()
		maps.Copy(h, it.headers)

		h.Set("Last-Modified", time.Unix(it.created, 0).UTC().Format(time.RFC1123))
		h.Set("Cache-Control", maxAge)
		return it.value
	}
}
