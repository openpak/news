// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/openpak/news/internal/content"
)

// source is what the internal API reads: the datasets nx-bcat renders into signed native BCAT.
type source interface {
	CurrentDataset(ctx context.Context, title string) (*content.Dataset, time.Time, error)
	OpenObject(ctx context.Context, key string) (io.ReadSeekCloser, int64, time.Time, error)
}

// registerInternal mounts nx-bcat's view of the datasets (README: nx-bcat). Strict X-Internal-Key:
// with no NX_INTERNAL_KEY configured every request answers 503.
func registerInternal(mux *http.ServeMux, src source) {
	guard := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			want := os.Getenv("NX_INTERNAL_KEY")
			switch {
			case want == "":
				http.Error(w, "internal key not configured", http.StatusServiceUnavailable)
			case !hmac.Equal([]byte(want), []byte(r.Header.Get("X-Internal-Key"))):
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			default:
				h(w, r)
			}
		}
	}
	mux.HandleFunc("GET /internal/datasets/{title}", guard(func(w http.ResponseWriter, r *http.Request) {
		d, mod, err := src.CurrentDataset(r.Context(), r.PathValue("title"))
		if errors.Is(err, content.ErrNotFound) {
			http.NotFound(w, r)
			return
		} else if err != nil {
			http.Error(w, "dataset unavailable", http.StatusInternalServerError)
			return
		}
		type file struct {
			Path   string `json:"path"`
			Key    string `json:"key"`
			Size   int64  `json:"size"`
			SHA256 string `json:"sha256"`
		}
		out := struct {
			TitleID    string     `json:"title_id"`
			ValidFrom  time.Time  `json:"valid_from"`
			ValidUntil *time.Time `json:"valid_until"`
			Files      []file     `json:"files"`
		}{d.TitleID, d.ValidFrom, d.ValidUntil, []file{}}
		for _, f := range d.Files {
			out.Files = append(out.Files, file{f.Path, f.Key, f.Size, f.SHA256})
		}
		w.Header().Set("Content-Type", "application/json")
		if !mod.IsZero() {
			w.Header().Set("Last-Modified", mod.UTC().Format(http.TimeFormat))
		}
		json.NewEncoder(w).Encode(out)
	}))
	mux.HandleFunc("GET /internal/objects/{key}", guard(func(w http.ResponseWriter, r *http.Request) {
		rc, _, mod, err := src.OpenObject(r.Context(), r.PathValue("key"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, "", mod, rc)
	}))
}
