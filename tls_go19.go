//go:build go1.9
// +build go1.9

package gserv

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
)

// RunTLS starts the server on the specific address, using tls
func (s *Server) RunTLS(addr string, certPairs []CertPair) error {
	cfg := tls.Config{RootCAs: x509.NewCertPool()}
	cfg.Certificates = make([]tls.Certificate, 0, len(certPairs))

	for _, cp := range certPairs {
		cert, err := tls.LoadX509KeyPair(cp.CertFile, cp.KeyFile)
		if err != nil {
			return fmt.Errorf("%s: %v", cp.CertFile, err)
		}
		cfg.Certificates = append(cfg.Certificates, cert)
	}

	if addr == "" {
		addr = ":https"
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	srv := s.newHTTPServer(context.Background(), ln.Addr().String(), false)
	srv.TLSConfig = &cfg

	s.serversMux.Lock()
	s.servers = append(s.servers, srv)
	s.serversMux.Unlock()

	return srv.ServeTLS(ln, "", "")
}
