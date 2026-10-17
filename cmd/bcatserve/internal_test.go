// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openpak/news/internal/content"
)

type fake struct{}

func (fake) CurrentDataset(_ context.Context, title string) (*content.Dataset, time.Time, error) {
	if title != "0100000000000001" {
		return nil, time.Time{}, content.ErrNotFound
	}
	return &content.Dataset{TitleID: title, Files: []content.DatasetFile{{Path: "a/b", Key: "k", Size: 3, SHA256: "ff"}}}, time.Unix(1700000000, 0), nil
}

type obj struct{ *bytes.Reader }

func (obj) Close() error { return nil }

func (fake) OpenObject(context.Context, string) (io.ReadSeekCloser, int64, time.Time, error) {
	return obj{bytes.NewReader([]byte("abc"))}, 3, time.Time{}, nil
}

func TestInternalDatasetAPI(t *testing.T) {
	mux := http.NewServeMux()
	registerInternal(mux, fake{})
	get := func(path, key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("X-Internal-Key", key)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	t.Setenv("NX_INTERNAL_KEY", "")
	if c := get("/internal/datasets/0100000000000001", "").Code; c != 503 {
		t.Fatalf("no key configured: %d", c)
	}
	t.Setenv("NX_INTERNAL_KEY", "k")
	if c := get("/internal/datasets/0100000000000001", "bad").Code; c != 401 {
		t.Fatalf("wrong key: %d", c)
	}
	if c := get("/internal/datasets/0100000000000002", "k").Code; c != 404 {
		t.Fatalf("unknown title: %d", c)
	}
	rec := get("/internal/datasets/0100000000000001", "k")
	if rec.Code != 200 || rec.Header().Get("Last-Modified") == "" || !bytes.Contains(rec.Body.Bytes(), []byte(`"sha256":"ff"`)) {
		t.Fatalf("dataset: %d %v %s", rec.Code, rec.Header(), rec.Body)
	}
	if rec := get("/internal/objects/k", "k"); rec.Body.String() != "abc" {
		t.Fatalf("object: %q", rec.Body)
	}
}
