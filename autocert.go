package gserv

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/idna"
)

// RunAutoCert enables automatic support for LetsEncrypt, using the optional passed domains list.
// certCacheDir is where the certificates will be cached, defaults to "./autocert".
// Note that it must always run on *BOTH* ":80" and ":443" so the addr param is omitted.
func (s *Server) RunAutoCert(ctx context.Context, certCacheDir string, domains ...string) error {
	var hbFn autocert.HostPolicy
	if len(domains) > 0 {
		hbFn = autocert.HostWhitelist(domains...)
	}

	return s.RunAutoCertDyn(ctx, certCacheDir, hbFn)
}

// RunAutoCertDyn enables automatic support for LetsEncrypt, using a dynamic HostPolicy.
// certCacheDir is where the certificates will be cached, defaults to "./autocert".
// Note that it must always run on *BOTH* ":80" and ":443" so the addr param is omitted.
func (s *Server) RunAutoCertDyn(ctx context.Context, certCacheDir string, hpFn autocert.HostPolicy) error {
	if certCacheDir == "" {
		certCacheDir = "./autocert"
	}

	if err := os.MkdirAll(certCacheDir, 0o700); err != nil {
		return fmt.Errorf("couldn't create cert cache dir: %v", err)
	}

	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(certCacheDir),
		HostPolicy: hpFn,
	}

	srv := s.newHTTPServer(ctx, ":https", false)

	tlsCfg := m.TLSConfig()
	tlsCfg.MinVersion = tls.VersionTLS12
	srv.TLSConfig = tlsCfg

	s.serversMux.Lock()
	s.servers = append(s.servers, srv)
	s.serversMux.Unlock()

	go func() {
		if err := http.ListenAndServe(":http", m.HTTPHandler(nil)); err != nil {
			s.Logf("gserv: autocert on :http error: %v", err)
		}
	}()

	return srv.ListenAndServeTLS("", "")
}

func NewAutoCertHosts(hosts ...string) *AutoCertHosts {
	return &AutoCertHosts{
		m: makeHosts(hosts...),
	}
}

type AutoCertHosts struct {
	m   map[string]struct{}
	mux sync.RWMutex
}

func (a *AutoCertHosts) Set(hosts ...string) {
	m := makeHosts(hosts...)
	a.mux.Lock()
	a.m = m
	a.mux.Unlock()
}

func makeHosts(hosts ...string) (m map[string]struct{}) {
	var e struct{}
	m = make(map[string]struct{}, len(hosts)+1)
	for _, h := range hosts {
		// copied from autocert.HostWhiteList
		if h, err := idna.Lookup.ToASCII(h); err == nil {
			m[h] = e
		}
	}
	return
}

func (a *AutoCertHosts) Contains(host string) bool {
	a.mux.RLock()
	_, ok := a.m[strings.ToLower(host)]
	a.mux.RUnlock()
	return ok
}

func (a *AutoCertHosts) IsAllowed(_ context.Context, host string) error {
	if a.Contains(host) {
		return nil
	}
	return fmt.Errorf("gserv/autocert: host %q not configured in AutoCertHosts", host)
}

// RunTLSAndAuto allows using custom certificates and autocert together.
// It will always listen on both :80 and :443
func (s *Server) RunTLSAndAuto(ctx context.Context, certCacheDir string, certPairs []CertPair, hosts *AutoCertHosts) error {
	if hosts == nil {
		return fmt.Errorf("gserv/autocert: hosts can't be nil")
	}

	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: hosts.IsAllowed,
	}

	m.HostPolicy = hosts.IsAllowed

	if certCacheDir == "" {
		certCacheDir = "./autocert"
	}

	if err := os.MkdirAll(certCacheDir, 0o700); err != nil {
		return fmt.Errorf("couldn't create cert cache dir (%s): %v", certCacheDir, err)
	}

	m.Cache = autocert.DirCache(certCacheDir)

	srv := s.newHTTPServer(ctx, ":https", false)

	cfg := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,

		NextProtos: []string{
			"h2", "http/1.1", // enable HTTP/2
			acme.ALPNProto, // enable tls-alpn ACME challenges
		},

		GetCertificate: m.GetCertificate,
	}

	for _, cp := range certPairs {
		cert, err := tls.LoadX509KeyPair(cp.CertFile, cp.KeyFile)
		if err != nil {
			return fmt.Errorf("%s: %v", cp.CertFile, err)
		}
		cfg.Certificates = append(cfg.Certificates, cert)
	}

	cfg.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		crt, err := m.GetCertificate(hello)
		if err == nil {
			return crt, err
		}

		// fallback to default impl tls impl
		return nil, nil
	}

	srv.TLSConfig = cfg

	s.serversMux.Lock()
	s.servers = append(s.servers, srv)
	s.serversMux.Unlock()

	ch := make(chan error, 2)

	go func() {
		if err := http.ListenAndServe(":80", m.HTTPHandler(nil)); err != nil {
			s.Logf("gserv: autocert on :80 error: %v", err)
			ch <- err
		}
	}()

	go func() {
		if err := srv.ListenAndServeTLS("", ""); err != nil {
			s.Logf("gserv: autocert on :443 error: %v", err)
			ch <- err
		}
	}()

	return <-ch
}
