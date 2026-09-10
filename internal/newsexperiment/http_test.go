// SPDX-License-Identifier: AGPL-3.0-only
package newsexperiment

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/openpak/news/internal/bcatcontainer"
)

func fixture(t *testing.T, dir, name string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := bcatcontainer.Encode([]byte{0x90}, make([]byte, 32), 0, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), payload, 0600); err != nil {
		t.Fatal(err)
	}
}

func writeTree(t *testing.T, topics ...string) string {
	t.Helper()
	dir := t.TempDir()
	fixture(t, dir, "catalog.bcat")
	for _, topic := range topics {
		sub := filepath.Join(dir, topic)
		if err := os.Mkdir(sub, 0755); err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{"detail.bcat", "icon.bcat", "archives.bcat"} {
			fixture(t, sub, f)
		}
	}
	return dir
}

func TestMultiChannelRoutes(t *testing.T) {
	dir := writeTree(t, "openpak_news", "openpak_games")
	h, err := Handler(dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	routes := []string{
		"/api/nx/v1/topics/catalog?l=en-GB&c%5B%5D=GB&c%5B%5D=GB",
		"/api/nx/v1/topics/openpak_news/detail",
		"/api/nx/v1/topics/openpak_news/icon",
		"/api/nx/v2/topics/openpak_news/online_archives?l=en-GB",
		"/api/nx/v1/topics/openpak_games/detail",
		"/api/nx/v1/topics/openpak_games/icon",
		"/api/nx/v2/topics/openpak_games/online_archives",
	}
	for _, path := range routes {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 200 || r.Header().Get("Content-Type") != "application/octet-stream" || r.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("bad response for %s: code=%d", path, r.Code)
		}
		if len(r.Body.Bytes()) <= 0x120 || string(r.Body.Bytes()[:4]) != "bcat" {
			t.Fatalf("not a container for %s", path)
		}
	}
	// Unknown topics and paths fail explicitly (no fallback configured).
	for _, path := range []string{
		"/api/nx/v1/topics/nx_news/detail",
		"/api/nx/v2/topics/openpak_help/online_archives",
		"/api/nx/v1/topics/openpak_games/icon/extra",
		"/catalog",
		"/api/nx/v1/topics/catalog/",
	} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 404 {
			t.Fatalf("unknown route %s accepted: %d", path, r.Code)
		}
	}
	// Prepared per-article artifacts are not routes.
	art := filepath.Join(dir, "openpak_news", "2609100001")
	if err := os.MkdirAll(art, 0755); err != nil {
		t.Fatal(err)
	}
	fixture(t, art, "article.bcat")
	h, err = Handler(dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/api/nx/v1/topics/openpak_news/2609100001/article", nil))
	if r.Code != 404 {
		t.Fatal("article container served as a route")
	}
}

func TestFallbackForUnknownTopic(t *testing.T) {
	dir := writeTree(t, "openpak_news")
	h, err := Handler(dir, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(418) }))
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/api/nx/v1/topics/nx_notice/detail", nil))
	if r.Code != 418 {
		t.Fatal("Nintendo-owned channel detail did not fall back")
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/other", nil))
	if r.Code != 418 {
		t.Fatal("fallback not used")
	}
}

func TestHandlerRejectsBadLayouts(t *testing.T) {
	t.Run("missing catalog", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Handler(dir, nil, nil); err == nil {
			t.Fatal("missing catalog accepted")
		}
	})
	t.Run("legacy single-channel layout", func(t *testing.T) {
		dir := writeTree(t, "openpak_news")
		fixture(t, dir, "detail.bcat")
		if _, err := Handler(dir, nil, nil); err == nil {
			t.Fatal("legacy flat fixture accepted")
		}
	})
	t.Run("incomplete channel", func(t *testing.T) {
		dir := writeTree(t, "openpak_news")
		if err := os.Remove(filepath.Join(dir, "openpak_news", "icon.bcat")); err != nil {
			t.Fatal(err)
		}
		if _, err := Handler(dir, nil, nil); err == nil {
			t.Fatal("channel without icon accepted")
		}
	})
	t.Run("invalid topic directory", func(t *testing.T) {
		dir := writeTree(t)
		if err := os.Mkdir(filepath.Join(dir, "..evil"), 0755); err != nil {
			t.Fatal(err)
		}
		if _, err := Handler(dir, nil, nil); err == nil {
			t.Fatal("invalid topic directory accepted")
		}
	})
	t.Run("no channels", func(t *testing.T) {
		dir := writeTree(t)
		if _, err := Handler(dir, nil, nil); err == nil {
			t.Fatal("catalog without channel directories accepted")
		}
	})
	t.Run("json fixture rejected", func(t *testing.T) {
		dir := writeTree(t, "openpak_news")
		if err := os.WriteFile(filepath.Join(dir, "openpak_news", "detail.bcat"), []byte(`{"topics":[]}`), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Handler(dir, nil, nil); err == nil {
			t.Fatal("JSON fixture accepted")
		}
	})
	t.Run("retired tree ignored", func(t *testing.T) {
		dir := writeTree(t, "openpak_news")
		ret := filepath.Join(dir, "retired", "openpak_news", "2609100001")
		if err := os.MkdirAll(ret, 0755); err != nil {
			t.Fatal(err)
		}
		if _, err := Handler(dir, nil, nil); err != nil {
			t.Fatalf("retired tree rejected: %v", err)
		}
	})
}

func TestServedBytesAreStable(t *testing.T) {
	dir := writeTree(t, "openpak_news")
	h, err := Handler(dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var first []byte
	for i := 0; i < 2; i++ {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", "/api/nx/v1/topics/openpak_news/detail", nil))
		if i == 0 {
			first = append([]byte(nil), r.Body.Bytes()...)
			continue
		}
		if !bytes.Equal(first, r.Body.Bytes()) {
			t.Fatal("fixture bytes changed between requests")
		}
	}
}
