// SPDX-License-Identifier: AGPL-3.0-only
package newsadmin

import (
	"archive/tar"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openpak/news/internal/newsexperiment"
)

// With nx-bcat configured a publish needs no signing key, serves no console tree, tells
// nx-bcat to pull, and exposes the unsigned seed for it to sign.
func TestPublishDelegatesToNxBcat(t *testing.T) {
	var syncs atomic.Int32
	var key atomic.Value
	bcat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		syncs.Add(1)
		key.Store(r.Header.Get("X-Internal-Key"))
	}))
	defer bcat.Close()
	svc, err := New(Config{DataDir: t.TempDir(), ImportDir: writeSourceTree(t), KeepBuilds: 3,
		Switch: &newsexperiment.Switch{}, VerifyKey: &signingKey(t).PublicKey,
		Bcat: &BcatSync{URL: bcat.URL, Key: "nx-key"}, Now: func() time.Time { return time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	api := svc.Handler(testKey)
	call := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("X-Internal-Key", testKey)
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}
	if rec := call("POST", "/admin/v1/publish"); rec.Code != 200 {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body)
	}
	svc.WaitPushes()
	if syncs.Load() != 1 || key.Load() != "nx-key" {
		t.Fatalf("nx-bcat syncs %d key %v", syncs.Load(), key.Load())
	}
	if rec := call("GET", "/admin/v1/bcat/current"); rec.Body.String() != `{"build":1,"revision":1}`+"\n" {
		t.Fatalf("current: %q", rec.Body)
	}
	rec := call("GET", "/admin/v1/bcat/builds/1/seed.tar")
	names := map[string]bool{}
	tr := tar.NewReader(rec.Body)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names[h.Name] = true
	}
	if !names["manifest.json"] || !names["catalog.msgpack"] {
		t.Fatalf("seed tar holds %v", names)
	}
	if rec := call("GET", "/admin/v1/bcat/builds/9/seed.tar"); rec.Code != 404 {
		t.Fatalf("unkept build: %d", rec.Code)
	}
}
