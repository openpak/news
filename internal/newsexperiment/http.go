// SPDX-License-Identifier: AGPL-3.0-only
// Package newsexperiment serves pre-signed original fixtures for hardware tests.
//
// The fixture directory layout mirrors cmd/newsseed's build output after
// signing (tools/news-cdn/sign-seed.sh):
//
//	catalog.bcat                    all channels, one signed catalog
//	<topic_id>/detail.bcat          per-channel detail record
//	<topic_id>/icon.bcat            per-channel icon (JPEG plaintext)
//	<topic_id>/archives.bcat        per-channel archives response
//
// Routes are derived from the directory, so a channel appears as soon as
// its three fixtures exist; the catalog is never decoded here because the
// CDN holds no keys. A tree with no channel directories serves the (empty)
// catalog alone — the builder's answer to "every channel retired/removed".
// Deeper subdirectories (per-article containers) and a root "retired" tree
// are prepared artifacts, not served routes. Unknown routes fail explicitly
// rather than inventing a successful response.
package newsexperiment

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/openpak/news/internal/bcatcontainer"
)

var reTopic = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)

// Handler keeps keys off the CDN and loads a bounded set of immutable fixtures.
func Handler(dir string, logger *slog.Logger, fallback http.Handler) (http.Handler, error) {
	if logger == nil {
		logger = slog.Default()
	}
	catalog, err := loadFixture(filepath.Join(dir, "catalog.bcat"))
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	// The pre-seed single-channel layout must not be mounted silently: it
	// would serve a catalog the per-channel routes cannot back.
	for _, legacy := range []string{"detail.bcat", "icon.bcat", "archives.bcat"} {
		if _, err := os.Stat(filepath.Join(dir, legacy)); err == nil {
			return nil, fmt.Errorf("legacy single-channel fixture %s found; rebuild with cmd/newsseed and sign with tools/news-cdn/sign-seed.sh", legacy)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type channel struct {
		detail, icon, archives []byte
		articles               map[string]map[string][]byte
	}
	channels := map[string]channel{}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "retired" {
			continue
		}
		topic := e.Name()
		if !reTopic.MatchString(topic) {
			return nil, fmt.Errorf("fixture directory %q is not a valid topic id", topic)
		}
		var ch channel
		for _, f := range []struct {
			name string
			dst  *[]byte
		}{{"detail.bcat", &ch.detail}, {"icon.bcat", &ch.icon}, {"archives.bcat", &ch.archives}} {
			b, err := loadFixture(filepath.Join(dir, topic, f.name))
			if err != nil {
				return nil, fmt.Errorf("channel %s: %w", topic, err)
			}
			*f.dst = b
		}
		ch.articles = map[string]map[string][]byte{}
		children, err := os.ReadDir(filepath.Join(dir, topic))
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			if !child.IsDir() {
				continue
			}
			id, err := strconv.ParseUint(child.Name(), 10, 32)
			if err != nil || id == 0 || strconv.FormatUint(id, 10) != child.Name() {
				return nil, fmt.Errorf("invalid article directory %s/%s", topic, child.Name())
			}
			resources := map[string][]byte{}
			for _, resource := range []string{"summary", "article"} {
				data, err := loadFixture(filepath.Join(dir, topic, child.Name(), resource+".bcat"))
				if err != nil {
					return nil, fmt.Errorf("article %s/%s: %w", topic, child.Name(), err)
				}
				resources[resource] = data
			}
			ch.articles[child.Name()] = resources
		}
		channels[topic] = ch
	}
	// An empty lineup is a legitimate state (every channel retired or
	// removed): the catalog fixture itself describes it, so serve it. Its
	// acceptance by the Find Channels UI on hardware is untested.
	if len(channels) == 0 {
		logger.Warn("no channel fixture directories; serving an empty catalog")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/nx/v1/topics/catalog", serveFixture(catalog, "catalog", logger))
	for topic, ch := range channels {
		mux.HandleFunc("GET /api/nx/v1/topics/"+topic+"/detail", serveFixture(ch.detail, topic+"/detail", logger))
		mux.HandleFunc("GET /api/nx/v1/topics/"+topic+"/icon", serveFixture(ch.icon, topic+"/icon", logger))
		mux.HandleFunc("GET /api/nx/v2/topics/"+topic+"/online_archives", serveFixture(ch.archives, topic+"/archives", logger))
		for id, resources := range ch.articles {
			for name, data := range resources {
				label := topic + "/" + id + "/" + name
				mux.HandleFunc("GET /openpak/v1/topics/"+topic+"/news/"+id+"/"+name, serveFixture(data, label, logger))
			}
		}
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("unhandled news route", "method", r.Method, "path", r.URL.Path)
		if fallback != nil {
			fallback.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
	return mux, nil
}

// loadFixture reads one container and validates the header fields the
// console's BCAT loader enforces, so a malformed file fails at start-up.
func loadFixture(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= bcatcontainer.HeaderSize || info.Size() > bcatcontainer.HeaderSize+bcatcontainer.MaxPayload {
		return nil, fmt.Errorf("invalid fixture size: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) <= bcatcontainer.HeaderSize || len(data) > bcatcontainer.HeaderSize+bcatcontainer.MaxPayload ||
		string(data[:4]) != "bcat" || data[4] != 1 || data[5] < 1 || data[5] > 3 || data[6] != 1 || data[7] > 15 {
		return nil, fmt.Errorf("invalid container header: %s", path)
	}
	return data, nil
}

func serveFixture(data []byte, label string, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-store")
		logger.Info("news fixture", "method", r.Method, "fixture", label, "bytes", len(data))
		http.ServeContent(w, r, label, time.Time{}, bytes.NewReader(data))
	}
}
