package sse

import (
	"bytes"
	"strconv"
	"sync/atomic"

	"go.oneofone.dev/gserv"
)

type Bidirectional[T any, OnRecvFn func(ctx *gserv.Context, data T) (out T, evt string, err error)] struct {
	sr *Router
	id uint64
}

func (b *Bidirectional[T, OnRecvFn]) Handler(paramName string) func(ctx *gserv.Context) gserv.Response {
	return func(ctx *gserv.Context) gserv.Response {
		return b.sr.Handle(ctx.Param(paramName), 16, ctx)
	}
}

func (b *Bidirectional[T, OnRecvFn]) InHandler(c gserv.Codec, paramName string, fn OnRecvFn) func(ctx *gserv.Context) gserv.Response {
	if c == nil {
		c = gserv.DefaultCodec
	}
	return func(ctx *gserv.Context) gserv.Response {
		var req T
		if err := ctx.Bind(nil, &req); err != nil {
			return gserv.NewJSONErrorResponse(400, err)
		}

		data, evt, err := fn(ctx, req)
		if err != nil {
			return gserv.NewJSONErrorResponse(400, err)
		}
		var buf bytes.Buffer
		if err := c.Encode(&buf, data); err != nil {
			return gserv.NewJSONErrorResponse(500, err)
		}
		evtID := strconv.FormatUint(atomic.AddUint64(&b.id, 1), 16)
		b.sr.Send(ctx.Param(paramName), evtID, evt, buf.Bytes())
		return gserv.RespEmpty
	}
}
