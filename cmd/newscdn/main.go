// SPDX-License-Identifier: AGPL-3.0-only
// newscdn serves pre-signed BCAT fixtures without access to content-signing keys.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"github.com/openpak/news/internal/newsexperiment"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:20040", "listen address")
	dir := flag.String("dir", "", "pre-signed fixture directory")
	cert := flag.String("tls-cert", "", "TLS certificate file")
	key := flag.String("tls-key", "", "TLS private key file")
	upstream := flag.String("fallback", "", "fixed HTTPS adapter upstream for other routes")
	ca := flag.String("fallback-ca", "", "adapter CA certificate file")
	serverName := flag.String("fallback-server-name", "", "adapter certificate DNS name")
	flag.Parse()
	if *dir == "" || (*cert == "") != (*key == "") {
		log.Fatal("require -dir and either both TLS files or neither")
	}
	var fallback http.Handler
	if *upstream != "" {
		u, err := url.Parse(*upstream)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || *serverName == "" {
			log.Fatal("fallback must be a fixed HTTPS origin with a certificate server name")
		}
		certs, err := os.ReadFile(*ca)
		if err != nil {
			log.Fatal(err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(certs) {
			log.Fatal("invalid fallback CA")
		}
		proxy := httputil.NewSingleHostReverseProxy(u)
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool, ServerName: *serverName}
		transport.ResponseHeaderTimeout = 10 * time.Second
		proxy.Transport = transport
		fallback = proxy
	}
	h, err := newsexperiment.Handler(*dir, nil, fallback)
	if err != nil {
		log.Fatal(err)
	}
	// Match the existing console adapter's non-hybrid curves and legacy TLS
	// support. This listener serves only the redirected console News hostname.
	suites := make([]uint16, 0, 32)
	for _, suite := range tls.CipherSuites() {
		suites = append(suites, suite.ID)
	}
	for _, suite := range tls.InsecureCipherSuites() {
		suites = append(suites, suite.ID)
	}
	tlsConfig := &tls.Config{
		MinVersion:       tls.VersionTLS10,
		CipherSuites:     suites,
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521},
		ClientAuth:       tls.NoClientCert,
		NextProtos:       []string{"http/1.1"},
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			log.Printf("News TLS hello versions=%v suites=%v", hello.SupportedVersions, hello.CipherSuites)
			return nil, nil
		},
	}
	srv := &http.Server{Addr: *listen, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10, TLSConfig: tlsConfig}
	// HTTP/2 disallows some of the legacy TLS suites this client needs.
	srv.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){}
	log.Printf("News fixture server listening on %s", *listen)
	if *cert != "" {
		err = srv.ListenAndServeTLS(*cert, *key)
	} else {
		err = srv.ListenAndServe()
	}
	log.Fatal(err)
}
