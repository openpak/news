package switchbcat

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openpak/news/internal/content"
	"github.com/openpak/news/render"
)

// fakeSource implements content.Reader in memory.
type fakeSource struct {
	articles []content.Article
	mod      time.Time
	titles   []string
	ds       *content.Dataset
	objs     map[string][]byte
}

func (f *fakeSource) Articles(ctx context.Context, q content.AudienceQuery) ([]content.Article, time.Time, error) {
	var out []content.Article
	for _, a := range f.articles {
		if a.Matches(q) {
			out = append(out, a)
		}
	}
	return out, f.mod, nil
}

func (f *fakeSource) Titles(ctx context.Context) ([]string, time.Time, error) {
	return f.titles, f.mod, nil
}

func (f *fakeSource) CurrentDataset(ctx context.Context, titleID string) (*content.Dataset, time.Time, error) {
	if f.ds == nil || f.ds.TitleID != titleID {
		return nil, time.Time{}, content.ErrNotFound
	}
	return f.ds, f.mod, nil
}

func (f *fakeSource) ImageByKey(ctx context.Context, key string) (*content.ArticleImage, time.Time, error) {
	if _, ok := f.objs[key]; !ok {
		return nil, time.Time{}, content.ErrNotFound
	}
	return &content.ArticleImage{Key: key}, f.mod, nil
}

func (f *fakeSource) OpenObject(ctx context.Context, key string) (io.ReadSeekCloser, int64, time.Time, error) {
	b, ok := f.objs[key]
	if !ok {
		return nil, 0, time.Time{}, content.ErrNotFound
	}
	return &nopCloser{bytes.NewReader(b)}, int64(len(b)), f.mod, nil
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

func get(t *testing.T, mux *http.ServeMux, path string, headers ...map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, hs := range headers {
		for k, v := range hs {
			req.Header.Set(k, v)
		}
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func newTestService(src *fakeSource) *http.ServeMux {
	return NewService(src, "https://bcat-data-lp1.cdn.nintendo.net", nil).Mux()
}

const titleID = "0100ed9024eb8000"

func TestEmulatorDelivery(t *testing.T) {
	src := &fakeSource{
		mod:    time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
		titles: []string{titleID},
		ds: &content.Dataset{
			ID: "d1", TitleID: titleID, ValidFrom: time.Now().UTC(), Sealed: true,
			Files: []content.DatasetFile{{Path: "data/event.bin", Key: "k1", Size: 3, SHA256: "aa"}},
		},
		objs: map[string][]byte{"k1": []byte("abc")},
	}
	mux := newTestService(src)

	rec := get(t, mux, "/api/emulator/v1/bcat/titles")
	if !strings.Contains(rec.Body.String(), titleID) {
		t.Fatalf("emu titles: %s", rec.Body.String())
	}
	rec = get(t, mux, "/api/emulator/v1/bcat/titles/"+titleID)
	if !strings.Contains(rec.Body.String(), "/api/emulator/v1/bcat/titles/"+titleID+"/files/data/event.bin") {
		t.Fatalf("emu manifest: %s", rec.Body.String())
	}
	if rec.Header().Get("ETag") == "" || rec.Header().Get("Last-Modified") == "" {
		t.Fatal("missing caching headers (NB-06)")
	}
	rec = get(t, mux, "/api/emulator/v1/bcat/titles/"+titleID+"/files/data/event.bin")
	if rec.Code != 200 || rec.Body.String() != "abc" {
		t.Fatalf("emu file: %d %q", rec.Code, rec.Body.String())
	}

	// Conditional fetch answers 304 (NB-06).
	etag := rec.Header().Get("ETag")
	rec = get(t, mux, "/api/emulator/v1/bcat/titles/"+titleID+"/files/data/event.bin",
		map[string]string{"If-None-Match": etag})
	if rec.Code != http.StatusNotModified {
		t.Fatalf("conditional file: %d", rec.Code)
	}

	if rec := get(t, mux, "/api/emulator/v1/bcat/titles/"+titleID+"/files/nope.bin"); rec.Code != 404 {
		t.Fatalf("unknown file: %d", rec.Code)
	}
	if rec := get(t, mux, "/api/emulator/v1/bcat/titles/0100deadbeef0000"); rec.Code != 404 {
		t.Fatalf("unknown title: %d", rec.Code)
	}
	// Path validation is defence in depth behind net/http's path cleaning.
	if content.ValidateFilePath("../etc/passwd") == nil {
		t.Fatal("traversal path must be rejected")
	}
}

func TestImageServing(t *testing.T) {
	src := &fakeSource{
		objs: map[string][]byte{"abc.jpg": []byte("jpegdata")},
		mod:  time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
	}
	mux := newTestService(src)

	rec := get(t, mux, "/images/abc.jpg")
	if rec.Code != 200 || rec.Body.String() != "jpegdata" || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("image: %d %q %s", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	etag := rec.Header().Get("ETag")
	rec = get(t, mux, "/images/abc.jpg", map[string]string{"If-None-Match": etag})
	if rec.Code != http.StatusNotModified {
		t.Fatalf("conditional image: %d", rec.Code)
	}
	if rec := get(t, mux, "/images/missing.png"); rec.Code != 404 {
		t.Fatalf("missing image: %d", rec.Code)
	}
}

func TestRendererContract(t *testing.T) {
	// NB-08: the Switch renderer is a render.Renderer (audience in,
	// channel-format out). After NB-00 the Switch channel's format is the
	// emulator delivery manifest, not console topics JSON.
	src := &fakeSource{
		ds: &content.Dataset{
			ID: "d1", TitleID: titleID, ValidFrom: time.Now().UTC(), Sealed: true,
			Files: []content.DatasetFile{{Path: "event/season.bin", Key: "k", Size: 1, SHA256: "ff"}},
		},
	}
	svc := NewService(src, "https://x.example", nil)
	var r render.Renderer = svc.Renderer()
	p, err := r.Render(context.Background(), content.AudienceQuery{Platform: "switch", TitleID: titleID})
	if err != nil {
		t.Fatal(err)
	}
	if r.Name() != "switch-bcat" || p.ContentType != "application/json" {
		t.Fatalf("payload meta: %+v", p)
	}
	if !bytes.Contains(p.Body, []byte(`"title_id":"`+titleID+`"`)) ||
		!bytes.Contains(p.Body, []byte("https://x.example/api/emulator/v1/bcat/titles/"+titleID+"/files/event/season.bin")) {
		t.Fatalf("payload body: %s", p.Body)
	}
	if p.ETag == "" {
		t.Fatal("payload must carry an ETag")
	}
	if _, err := r.Render(context.Background(), content.AudienceQuery{Platform: "switch"}); err == nil {
		t.Fatal("renderer must reject title-less queries")
	}
}
