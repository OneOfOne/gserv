package gserv

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"go.oneofone.dev/otk"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/idna"
)

func NewCertPair(certFile, keyFile string) (cp CertPair, err error) {
	var cert, key []byte
	if cert, err = os.ReadFile(certFile); err != nil {
		return
	}

	if key, err = os.ReadFile(keyFile); err != nil {
		return
	}

	return CertPair{Cert: cert, Key: key}, nil
}

// CertPair is a pair of (cert, key) files to listen on TLS
type CertPair struct {
	Cert  []byte   `json:"cert"`
	Key   []byte   `json:"key"`
	Roots [][]byte `json:"roots"`
}

// RunAutoCert enables automatic support for LetsEncrypt, using the optional passed domains list.
// certCacheDir is where the certificates will be cached, defaults to "./autocert".
// Note that it must always run on *BOTH* ":80" and ":443" so the addr param is omitted.
func (s *Server) RunAutoCert(ctx context.Context, certCacheDir string, domains ...string) error {
	var hbFn autocert.HostPolicy
	if len(domains) > 0 {
		hbFn = autocert.HostWhitelist(domains...)
	}

	return s.RunAutoCertDyn(ctx, &AutoCertOpts{
		CacheDir: certCacheDir,
		Hosts:    hbFn,
	})
}

type AutoCertOpts struct {
	Hosts autocert.HostPolicy `json:"hosts"`

	Eab *acme.ExternalAccountBinding `json:"eab"`

	Email    string `json:"email"`
	CacheDir string `json:"cacheDir"`

	DirectoryURL string `json:"directoryURL"`
}

func (aco *AutoCertOpts) manager() (*autocert.Manager, error) {
	if aco == nil {
		aco = &AutoCertOpts{}
	}

	if aco.CacheDir == "" {
		aco.CacheDir = "./autocert"
	}

	if err := os.MkdirAll(aco.CacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("gserv/autocert: couldn't create cert cache dir (%s): %w", aco.CacheDir, err)
	}

	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(aco.CacheDir),
		Email:      aco.Email,
		HostPolicy: aco.Hosts,
	}

	if aco.DirectoryURL != "" {
		m.Client = &acme.Client{
			DirectoryURL: aco.DirectoryURL,
		}
	}

	if aco.Eab != nil {
		m.ExternalAccountBinding = aco.Eab
	}

	return m, nil
}

// RunAutoCertDyn enables automatic support for LetsEncrypt, using a dynamic HostPolicy.
// certCacheDir is where the certificates will be cached, defaults to "./autocert".
// Note that it must always run on *BOTH* ":80" and ":443" so the addr param is omitted.
func (s *Server) RunAutoCertDyn(ctx context.Context, opts *AutoCertOpts) error {
	m, err := opts.manager()
	if err != nil {
		return err
	}
	srv := s.newHTTPServer(ctx, ":https", false)

	tlsCfg := m.TLSConfig()
	tlsCfg.MinVersion = tls.VersionTLS12
	srv.TLSConfig = tlsCfg

	s.serversMux.Lock()
	s.servers = append(s.servers, srv)
	s.serversMux.Unlock()

	go func() {
		if err := http.ListenAndServe(":http", m.HTTPHandler(nil)); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.Logf("gserv/autocert: error: %v", err)
		}
	}()

	if err = srv.ListenAndServeTLS("", ""); errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}

func NewAutoCertHosts(hosts ...string) *AutoCertHosts {
	var ach AutoCertHosts
	ach.appendHosts(hosts...)
	return &ach
}

type AutoCertHosts struct {
	m   otk.Set
	mux sync.RWMutex
}

func (a *AutoCertHosts) Set(hosts ...string) {
	a.mux.Lock()
	a.appendHosts(hosts...)
	a.mux.Unlock()
}

func (a *AutoCertHosts) appendHosts(hosts ...string) (m map[string]struct{}) {
	for _, h := range hosts {
		// copied from autocert.HostWhiteList
		if h, err := idna.Lookup.ToASCII(h); err == nil {
			a.m.Set(h)
		}
	}
	return
}

func (a *AutoCertHosts) Contains(host string) bool {
	host = strings.ToLower(host)
	if h, err := idna.Lookup.ToASCII(host); err == nil {
		host = h
	} else {
		return false
	}
	a.mux.RLock()
	ok := a.m.Has(host)
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
func (s *Server) RunTLSAndAuto(ctx context.Context, certPairs []CertPair, opts *AutoCertOpts) error {
	m, err := opts.manager()
	if err != nil {
		return err
	}
	srv := s.newHTTPServer(ctx, ":https", false)

	cfg := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.X25519, // Go 1.8 only
		},
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, // Go 1.8 only
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,   // Go 1.8 only
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,

			// Best disabled, as they don't provide Forward Secrecy,
			// but might be necessary for some clients
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		},
		NextProtos: []string{
			"h2", "http/1.1", // enable HTTP/2
			acme.ALPNProto, // enable tls-alpn ACME challenges
		},
	}

	for _, cp := range certPairs {
		var cert tls.Certificate
		var err error
		cert, err = tls.X509KeyPair(cp.Cert, cp.Key)
		if err != nil {
			return err
		}
		cfg.Certificates = append(cfg.Certificates, cert)
		if len(cp.Roots) > 0 {
			if cfg.RootCAs == nil {
				cfg.RootCAs = x509.NewCertPool()
			}
			for _, crt := range cp.Roots {
				cfg.RootCAs.AppendCertsFromPEM(crt)
			}
		}
	}

	cfg.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if m.HostPolicy != nil {
			crt, err := m.GetCertificate(hello)
			if err == nil {
				return crt, err
			}
		}
		// fallback to default tls impl
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
