package gserv

import (
	"errors"
	"io"
	"net/http"
)

type GroupType interface {
	AddRoute(method, path string, handlers ...Handler) Route
}

func Get[CodecT Codec, Resp any, HandlerFn func(ctx *Context) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return handleOutOnly[CodecT](g, http.MethodGet, path, handler, wrapResp)
}

func JSONGet[Resp any, HandlerFn func(ctx *Context) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return Get[JSONCodec](g, path, handler, wrapResp)
}

func MsgpGet[Resp any, HandlerFn func(ctx *Context) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return Get[MsgpCodec](g, path, handler, wrapResp)
}

func Delete[CodecT Codec, Resp any, HandlerFn func(ctx *Context) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return handleOutOnly[CodecT](g, http.MethodDelete, path, handler, wrapResp)
}

func JSONDelete[Resp any, HandlerFn func(ctx *Context) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return Delete[JSONCodec](g, path, handler, wrapResp)
}

func MsgpDelete[Resp any, HandlerFn func(ctx *Context) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return Delete[MsgpCodec](g, path, handler, wrapResp)
}

func Post[CodecT Codec, Req, Resp any, HandlerFn func(ctx *Context, reqBody Req) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return handleInOut[CodecT](g, http.MethodPost, path, handler, wrapResp)
}

func JSONPost[Req, Resp any, HandlerFn func(ctx *Context, reqBody Req) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return Post[JSONCodec](g, path, handler, wrapResp)
}

func MsgpPost[Req, Resp any, HandlerFn func(ctx *Context, reqBody Req) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return Post[MsgpCodec](g, path, handler, wrapResp)
}

func Put[CodecT Codec, Req, Resp any, HandlerFn func(ctx *Context, reqBody Req) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return handleInOut[CodecT](g, http.MethodPut, path, handler, wrapResp)
}

func JSONPut[Req, Resp any, HandlerFn func(ctx *Context, reqBody Req) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return Put[JSONCodec](g, path, handler, wrapResp)
}

func MsgpPut[Req, Resp any, HandlerFn func(ctx *Context, reqBody Req) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return Put[MsgpCodec](g, path, handler, wrapResp)
}

func Patch[CodecT Codec, Req, Resp any, HandlerFn func(ctx *Context, reqBody Req) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return handleInOut[CodecT](g, http.MethodPatch, path, handler, wrapResp)
}

func JSONPatch[Req, Resp any, HandlerFn func(ctx *Context, reqBody Req) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return Patch[JSONCodec](g, path, handler, wrapResp)
}

func MsgpPatch[Req, Resp any, HandlerFn func(ctx *Context, reqBody Req) (resp Resp, err error)](g GroupType, path string, handler HandlerFn, wrapResp bool) Route {
	return Patch[MsgpCodec](g, path, handler, wrapResp)
}

func handleOutOnly[CodecT Codec, Resp any, HandlerFn func(ctx *Context) (resp Resp, err error)](g GroupType, method, path string, handler HandlerFn, wrapResp bool) Route {
	var c CodecT
	var resp Resp
	_, respBytes := any(resp).([]byte)

	return g.AddRoute(method, path, func(ctx *Context) Response {
		resp, err := handler(ctx)
		if err != nil {
			return handleError[CodecT](ctx, err, wrapResp)
		}
		if wrapResp {
			return NewResponse[CodecT](resp)
		}
		if respBytes {
			_, _ = ctx.Write(any(resp).([]byte))
			return nil
		}
		_ = c.Encode(ctx, resp)
		return nil
	})
}

func handleInOut[CodecT Codec, Req, Resp any, HandlerFn func(ctx *Context, reqBody Req) (resp Resp, err error)](g GroupType, method, path string, handler HandlerFn, wrapResp bool) Route {
	var c CodecT
	var req Req
	var resp Resp
	_, reqBytes := any(req).([]byte)
	_, respBytes := any(resp).([]byte)
	return g.AddRoute(method, path, func(ctx *Context) Response {
		var body Req
		if reqBytes {
			b, err := io.ReadAll(ctx.Req.Body)
			if err != nil {
				return handleError[CodecT](ctx, err, wrapResp)
			}
			*(any(&body).(*[]byte)) = b
		} else if err := c.Decode(ctx.Req.Body, &body); err != nil && !errors.Is(err, io.EOF) {
			return handleError[CodecT](ctx, err, wrapResp)
		}

		ctx.SetContentType(c.ContentType())
		resp, err := handler(ctx, body)
		if err != nil {
			return handleError[CodecT](ctx, err, wrapResp)
		}
		if wrapResp {
			return NewResponse[CodecT](resp)
		}
		if respBytes {
			_, _ = ctx.Write(any(resp).([]byte))
			return nil
		}
		_ = c.Encode(ctx, resp)
		return nil
	})
}

func handleError[C Codec](ctx *Context, e error, wrapResp bool) Response {
	var c C
	err := getError(e)
	if wrapResp {
		return NewErrorResponse[C](err.Status(), err)
	}
	ctx.WriteHeader(err.Status())
	_ = c.Encode(ctx, getError(err))
	return nil
}
