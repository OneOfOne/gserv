package gserv

import (
	"log"
	"time"

	"go.oneofone.dev/gserv/router"
)

// Options contains configuration options for the Server.
type Options struct {
	Logger         *log.Logger
	RouterOptions  *router.Options
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	MaxHeaderBytes int

	CatchPanics bool
}

// Option is a functional option type to configure the Server.
type Option = func(opt *Options)

// ReadTimeout sets the read timeout on the server, equivalent to http.Server.ReadTimeout.
func ReadTimeout(v time.Duration) Option {
	return func(opt *Options) {
		opt.ReadTimeout = v
	}
}

// WriteTimeout sets the write timeout on the server, equivalent to http.Server.WriteTimeout.
func WriteTimeout(v time.Duration) Option {
	return func(opt *Options) {
		opt.WriteTimeout = v
	}
}

// MaxHeaderBytes sets the maximum size of request headers on the server, equivalent to http.Server.MaxHeaderBytes.
func MaxHeaderBytes(v int) Option {
	return func(opt *Options) {
		opt.MaxHeaderBytes = v
	}
}

// SetErrLogger sets the error logger for the server, equivalent to http.Server.ErrorLog.
func SetErrLogger(v *log.Logger) Option {
	return func(opt *Options) {
		opt.Logger = v
	}
}

// SetRouterOptions configures the underlying router options.
func SetRouterOptions(v *router.Options) Option {
	return func(opt *Options) {
		opt.RouterOptions = v
	}
}

// SetCatchPanics enables or disables panic recovery in handlers.
func SetCatchPanics(enable bool) Option {
	return func(opt *Options) {
		opt.CatchPanics = enable
	}
}

// SetProfileLabels enables or disables profile labels on the underlying router.
func SetProfileLabels(enable bool) Option {
	return func(opt *Options) {
		opt.RouterOptions.ProfileLabels = enable
	}
}

// SetOnReqDone sets a callback to be invoked after each request is processed by the router.
func SetOnReqDone(fn router.OnRequestDone) Option {
	return func(opt *Options) {
		opt.RouterOptions.OnRequestDone = fn
	}
}
