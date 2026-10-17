// SPDX-License-Identifier: AGPL-3.0-only
// newscdn serves signed BCAT fixtures to consoles. Without -data it serves
// one pre-signed tree and holds no keys. With -data it also runs the admin
// API (docs/admin-api.md) on a separate plain-HTTP listener: channels and
// articles are authored there, and a publish signs a new tree in-process
// (keys from NEWS_SIGNING_KEY / NEWS_AES_KEY) and switches to it without a
// restart.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/openpak/news/internal/newsadmin"
	"github.com/openpak/news/internal/newsexperiment"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:20040", "listen address")
	dir := flag.String("dir", "", "pre-signed fixture directory")
	cert := flag.String("tls-cert", "", "TLS certificate file")
	key := flag.String("tls-key", "", "TLS private key file")
	upstream := flag.String("fallback", "", "fixed HTTPS adapter upstream for other routes")
	ca := flag.String("fallback-ca", "", "adapter CA certificate file")
	serverName := flag.String("fallback-server-name", "", "adapter certificate DNS name")
	data := flag.String("data", "", "writable data directory: enables authoring, publishing and the admin API")
	importDir := flag.String("import", "", "channels/ directory imported into an empty -data")
	importShots := flag.Bool("import-screenshots", false, "also import the channels/screenshots/ captures")
	importManifest := flag.String("import-manifest", "", "manifest.json of the last file-based build, for overwrite continuity")
	adminListen := flag.String("admin-listen", ":20043", "plain-HTTP admin API listener (internal network only)")
	keep := flag.Int("keep-builds", 10, "published builds to keep (minimum 2)")
	verifyKey := flag.String("verify-key", "", "public key PEM to verify containers against (default: the committed OpenPak console key)")
	flag.Parse()
	if (*dir == "" && *data == "") || (*cert == "") != (*key == "") {
		log.Fatal("require -dir or -data, and either both TLS files or neither")
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
	var h http.Handler
	var err error
	if *data == "" {
		h, err = newsexperiment.Handler(*dir, nil, fallback)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		h = startAdmin(*data, *dir, *importDir, *importShots, *importManifest, *adminListen, *keep, *verifyKey, fallback)
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

// startAdmin opens the data directory, starts the admin listener and
// returns the switchable console handler. Secrets come from the
// environment only and are never logged.
func startAdmin(data, static, importDir string, importShots bool, importManifest, listen string, keep int, verifyKey string, fallback http.Handler) http.Handler {
	sw := &newsexperiment.Switch{}
	cfg := newsadmin.Config{
		DataDir: data, ImportDir: importDir, ImportScreenshots: importShots, ImportManifest: importManifest,
		SigningKeyPath: os.Getenv("NEWS_SIGNING_KEY"), AESKeyPath: os.Getenv("NEWS_AES_KEY"),
		KeepBuilds: keep, StaticDir: static, Fallback: fallback, Switch: sw,
	}
	if verifyKey != "" {
		raw, err := os.ReadFile(verifyKey)
		if err != nil {
			log.Fatal(err)
		}
		if cfg.VerifyKey, err = newsadmin.ParseVerifyKey(raw); err != nil {
			log.Fatal(err)
		}
	}
	if u := os.Getenv("NX_BAAS_PUSH_URL"); u != "" {
		wait, _ := strconv.Atoi(os.Getenv("NX_BAAS_PUSH_WAIT_RANGE"))
		cfg.Push = &newsadmin.Pusher{URL: u, Key: os.Getenv("NX_BAAS_INTERNAL_KEY"), WaitRange: wait}
	}
	if u := os.Getenv("NX_BCAT_SYNC_URL"); u != "" {
		cfg.Bcat = &newsadmin.BcatSync{URL: u, Key: os.Getenv("NX_INTERNAL_KEY")}
	}
	svc, err := newsadmin.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	adminKey := os.Getenv("NEWS_ADMIN_KEY")
	if adminKey == "" {
		slog.Warn("NEWS_ADMIN_KEY is not set: the admin API answers 503 to everything")
	}
	if (cfg.SigningKeyPath == "" || cfg.AESKeyPath == "") && cfg.Bcat == nil {
		slog.Warn("NEWS_SIGNING_KEY / NEWS_AES_KEY not set: publishing is disabled")
	}
	admin := &http.Server{Addr: listen, Handler: svc.Handler(adminKey), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: time.Minute, WriteTimeout: 5 * time.Minute, IdleTimeout: time.Minute, MaxHeaderBytes: 16 << 10}
	go func() {
		log.Printf("News admin API listening on %s (plain HTTP, internal network only)", listen)
		log.Fatal(admin.ListenAndServe())
	}()
	return sw
}
