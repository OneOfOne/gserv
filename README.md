# gserv

[![Go Reference](https://pkg.go.dev/badge/go.oneofone.dev/gserv.svg)](https://pkg.go.dev/go.oneofone.dev/gserv)

A simple, fast, and flexible HTTP server framework for Go.

## Features

- **Zero dependencies on HTTP routing** -- ships with its own lightweight router (`gserv/router`)
- **HTTP/2 support** -- enabled automatically via H2C
- **Multiple codecs** -- built-in JSON and MessagePack serialization
- **SSE (Server-Sent Events)** -- first-class support via `gserv/sse`
- **Gzip compression** -- automatic when the client accepts gzip
- **Caching middleware** -- ETag-based response caching with configurable TTL
- **Rate limiting middleware** -- per-key limits at second, minute, and hour granularity
- **Group-based routing** -- organized route registration with inherited middleware chains
- **Panic recovery** -- optional panic handler integration via `oerrs` frame capture

## Installation

```bash
go get go.oneofone.dev/gserv
```

## Quick Start

### Basic Server

```go
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.oneofone.dev/gserv"
)

func main() {
	srv := gserv.New()

	srv.GET("/health", func(ctx *gserv.Context) gserv.Response {
		return gserv.NewJSONResponse("OK")
	})

	srv.GET("/users/:id", func(ctx *gserv.Context) gserv.Response {
		id := ctx.Param("id")
		return gserv.NewJSONResponse(map[string]string{"id": id})
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		if err := srv.Run(ctx, ":8080"); err != nil {
			println(err.Error())
		}
	}()

	<-ctx.Done()
	srv.Shutdown(5 * time.Second)
}
```

### Grouped Routes with Middleware

```go
api := srv.SubGroup("api", "/api", gserv.LogRequests(false))

users := api.SubGroup("users", "/users")
users.Use(authMiddleware) // inherited by all routes in this subgroup

users.GET("", listUsers)
users.GET("/:id", getUser)
users.POST("", createUser)
users.DELETE("/:id", deleteUser)
```

### Typed JSON Responses

```go
func getUser(ctx *gserv.Context) gserv.Response {
	id := ctx.Param("id")

	user, err := db.FindUser(id)
	if err != nil {
		return gserv.NewJSONErrorResponse(http.StatusInternalServerError, err)
	}

	if user == nil {
		return gserv.NewJSONErrorResponse(http.StatusNotFound, "user not found")
	}

	return gserv.NewJSONResponse(user)
}
```

### Request Binding (JSON and MessagePack)

```go
func createUser(ctx *gserv.Context) gserv.Response {
	var req struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	if err := ctx.Bind(&req); err != nil {
		return gserv.NewJSONErrorResponse(http.StatusBadRequest, err)
	}

	// ... handle request
	return gserv.NewJSONResponse(req)
}
```

### Server-Sent Events (SSE)

```go
import "go.oneofone.dev/gserv/sse"

sseRouter := sse.NewRouter()

srv.GET("/stream", func(ctx *gserv.Context) gserv.Response {
	return sseRouter.Handle("channel1", 256, ctx)
})

// Publish events from anywhere:
go func() {
	for {
		sseRouter.Send("channel1", "", "message", map[string]string{"text": "hello"})
		time.Sleep(time.Second)
	}
}()
```

### Caching Middleware

```go
srv.GET("/products", gserv.CacheHandler(
	func(ctx *gserv.Context) string {
		return fmt.Sprintf("products:lang=%s", ctx.QueryDefault("lang", "en"))
	},
	5*time.Minute, // cache TTL
	listProducts,  // cached handler
))
```

### Rate Limiting Middleware

```go
// Limits: 10/second, 100/minute, 1000/hour per client IP
rateLimiter := gserv.RateLimiter(ctx, nil, 10, 100, 1000, true)

users.Use(rateLimiter)
```

### Static Files

```go
srv.Static("/static", "./public", false)       // directory serving
srv.StaticFile("/favicon.ico", "./assets/ico") // single file
```

## Response Types

| Type | Content-Type | Usage |
|------|-------------|-------|
| `gserv.NewJSONResponse(data)` | `application/json` | Standard JSON API response |
| `gserv.NewMsgpResponse(data)` | `application/msgpack` | MessagePack serialization |
| `gserv.NewJSONErrorResponse(code, err)` | `application/json` | Error response with stack |
| `gserv.RespOK` | `text/plain` | Cached 200 OK |
| `gserv.RespNotFound` | `application/json` | Cached 404 |
| `gserv.File(ct, path)` | varies | Serve a file |

## Context API

| Method | Description |
|--------|-------------|
| `ctx.Param(key)` | URL path parameter |
| `ctx.Query(key)` | Query string parameter |
| `ctx.Bind(&v)` | Bind request body (auto-detects JSON/MsgPack) |
| `ctx.JSON(code, v)` | Write JSON response directly |
| `ctx.Msgpack(code, v)` | Write MsgPack response directly |
| `ctx.Get(key)`, `ctx.Set(key, val)` | Typed context values |
| `ctx.ClientIP()` | Client IP (respects X-Real-Ip / X-Forwarded-For) |
| `ctx.File(path)` | Serve a file |
| `ctx.SetCookie(...)` | Set signed http-only cookie |

## Server Configuration

```go
srv := gserv.New(
	gserv.ReadTimeout(time.Second*30),
	gserv.WriteTimeout(time.Minute),
	gserv.MaxHeaderBytes(1<<20),
	gserv.SetErrLogger(myLogger),
	gserv.SetCatchPanics(true),
)
```

## Trusted By

- Powering [aiq.com](https://aiq.com) since 2019.


## Disclaimer

- AI was used to generate this README and some of the package's documentation.

## License

[MIT](LICENSE)
