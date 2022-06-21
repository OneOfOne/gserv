package gserv

import (
	"log"
	"time"

	"go.oneofone.dev/gserv/router"
)

// Options allows finer control over the gserv
type Options struct {
	Logger         *log.Logger
	RouterOptions  *router.Options
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	MaxHeaderBytes int

	CatchPanics              bool
	EnableDefaultHTTPLogging bool // disables the spam on disconnects and tls, it can hide important messages sometimes
}

// Option is a func to set internal server Options.
type Option = func(opt *Options)

// ReadTimeout sets the read timeout on the server.
// see http.Server.ReadTimeout
func ReadTimeout(v time.Duration) Option {
	return func(opt *Options) {
		opt.ReadTimeout = v
	}
}

// WriteTimeout sets the write timeout on the server.
// see http.Server.WriteTimeout
func WriteTimeout(v time.Duration) Option {
	return func(opt *Options) {
		opt.WriteTimeout = v
	}
}

// MaxHeaderBytes sets the max size of headers on the server.
// see http.Server.MaxHeaderBytes
func MaxHeaderBytes(v int) Option {
	return func(opt *Options) {
		opt.MaxHeaderBytes = v
	}
}

// SetErrLogger sets the error logger on the server.
func SetErrLogger(v *log.Logger) Option {
	return func(opt *Options) {
		opt.Logger = v
	}
}

// SetRouterOptions sets gserv/router.Options on the server.
func SetRouterOptions(v *router.Options) Option {
	return func(opt *Options) {
		opt.RouterOptions = v
	}
}

// SetNoCatchPanics toggles catching panics in handlers.
func SetCatchPanics(enable bool) Option {
	return func(opt *Options) {
		opt.CatchPanics = enable
	}
}

func SetProfileLabels(enable bool) Option {
	return func(opt *Options) {
		opt.RouterOptions.ProfileLabels = enable
	}
}

func SetOnReqDone(fn router.OnRequestDone) Option {
	return func(opt *Options) {
		opt.RouterOptions.OnRequestDone = fn
	}
}
