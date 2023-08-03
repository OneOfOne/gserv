package apiutils

import (
	"net/http"

	"go.oneofone.dev/gserv"
)

// SHM is a Simple Headers Map
type SHM map[string]string

// Apply applies the SHM to an http.Header, if add is true, the values gets Added rather than Set
// if a value is empty, it gets deleted
func (m SHM) Apply(hh http.Header, add bool) {
	for k, v := range m {
		switch {
		case v == "":
			hh.Del(k)
		case add:
			hh.Add(k, v)
		default:
			hh.Set(k, v)
		}
	}
}

// Set sets a key to a value
func (m SHM) Set(k, v string) SHM {
	m[k] = v
	return m
}

// Copy returns a copy of the map
func (m SHM) Copy() SHM {
	cp := make(SHM, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

// SimpleHeadersMaps of common headers based on https://rorsecurity.info/portfolio/new-http-headers-for-more-security
var (
	SecureHeaders = SHM{
		"X-Frame-Options":  "SAMEORIGIN",
		"X-XSS-Protection": "1; mode=block",
		// IE security
		"X-Download-Options": "noopen",

		// https://scotthelme.co.uk/content-security-policy-an-introduction/
		"Content-Security-Policy": "default-src https:",
	}

	// https://googleblog.blogspot.com/2007/07/robots-exclusion-protocol-now-with-even.html
	NoIndexing = SHM{
		"X-Robots-Tag": "noindex",
	}

	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Strict-Transport-Security
	HSTS = SHM{
		"Strict-Transport-Security": "max-age=15552000; includeSubDomains",
	}

	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Strict-Transport-Security
	HSTSPreload = SHM{
		"Strict-Transport-Security": "max-age=15552000; includeSubDomains; preload",
	}
)

// ApplyHeaders is a middle to apply a static set of headers to an gserv.Context
func ApplyHeaders(headerMaps ...SHM) gserv.Handler {
	return func(ctx *gserv.Context) error {
		ch := ctx.Header()
		for _, hm := range headerMaps {
			hm.Apply(ch, false)
		}
		return nil
	}
}
