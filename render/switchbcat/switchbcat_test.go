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

func testArticle(slug string, aud content.Audience, locales ...content.ArticleLocale) content.Article {
	pub := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	return content.Article{ID: "11111111-1111-1111-1111-11111111111" + slug[:1], Slug: slug, Audience: aud, PublishedAt: &pub, Locales: locales}
}

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

func TestTopicsCountryAndTitleFilters(t *testing.T) {
	src := &fakeSource{mod: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)}
	src.articles = []content.Article{
		testArticle("global", content.Audience{}, // all platforms, all titles, all regions
			content.ArticleLocale{Language: "en-US", Title: "OpenPak is live", Body: "Welcome.\n\nSecond para"}),
		testArticle("stardew-fr", content.Audience{TitleIDs: []string{titleID}, Regions: []string{"FR"}},
			content.ArticleLocale{Language: "fr-FR", Title: "Serveur Stardew", Body: "Bonjour"}),
	}
	mux := newTestService(src)

	rec := get(t, mux, "/api/nx/v1/titles/"+titleID+"/topics?c[]=FR&c[]=GB")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"OpenPak is live"`) || !strings.Contains(body, `"Serveur Stardew"`) {
		t.Fatalf("missing topics: %s", body)
	}
	if !strings.Contains(body, `"title_id":"`+titleID+`"`) {
		t.Fatalf("missing title_id: %s", body)
	}
	if !strings.Contains(body, `"paraphrase":"Welcome."`) {
		t.Fatalf("missing paraphrase: %s", body)
	}
	if rec.Header().Get("ETag") == "" || rec.Header().Get("Last-Modified") == "" {
		t.Fatal("missing caching headers (NB-06)")
	}

	// Region filter excludes the FR-only article for c[]=US.
	rec = get(t, mux, "/api/nx/v1/titles/"+titleID+"/topics?c[]=US")
	if strings.Contains(rec.Body.String(), "Serveur Stardew") {
		t.Fatal("FR-only article leaked to US request")
	}
	if !strings.Contains(rec.Body.String(), "OpenPak is live") {
		t.Fatal("channel-wide article must reach every region")
	}

	// A different title sees only channel-wide articles.
	rec = get(t, mux, "/api/nx/v1/titles/0100000000010000/topics?c[]=FR")
	if strings.Contains(rec.Body.String(), "Serveur Stardew") {
		t.Fatal("per-title article leaked to another title")
	}
}

func TestTopicsLanguageTargetingAndLocalePick(t *testing.T) {
	src := &fakeSource{}
	src.articles = []content.Article{
		testArticle("langs", content.Audience{Languages: []string{"en-US", "fr-FR"}},
			content.ArticleLocale{Language: "en-US", Title: "Hello", Body: "Hi"},
			content.ArticleLocale{Language: "fr-FR", Title: "Salut", Body: "Bonjour"}),
		testArticle("enonly", content.Audience{Languages: []string{"en-US"}},
			content.ArticleLocale{Language: "en-US", Title: "EN only", Body: "x"}),
	}
	mux := newTestService(src)

	rec := get(t, mux, "/api/nx/v1/titles/"+titleID+"/topics?c[]=US&l=fr-FR")
	body := rec.Body.String()
	if !strings.Contains(body, `"Salut"`) {
		t.Fatalf("requested language not served: %s", body)
	}
	if strings.Contains(body, "EN only") {
		t.Fatal("language-targeted article leaked to disallowed language")
	}

	rec = get(t, mux, "/api/nx/v1/titles/"+titleID+"/topics?c[]=US&l=de-DE")
	if strings.Contains(rec.Body.String(), `"topics":[{`) {
		t.Fatalf("language-targeted article must not serve to an untargeted language: %s", rec.Body.String())
	}

	// No language in the request: first locale is served.
	rec = get(t, mux, "/api/nx/v1/titles/"+titleID+"/topics?c[]=US")
	if !strings.Contains(rec.Body.String(), `"Hello"`) {
		t.Fatalf("fallback to first locale expected: %s", rec.Body.String())
	}
}

func TestTopicsETagConditional(t *testing.T) {
	src := &fakeSource{mod: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)}
	src.articles = []content.Article{testArticle("global", content.Audience{},
		content.ArticleLocale{Language: "en-US", Title: "t", Body: "b"})}
	mux := newTestService(src)

	first := get(t, mux, "/api/nx/v1/titles/"+titleID+"/topics")
	etag := first.Header().Get("ETag")
	second := get(t, mux, "/api/nx/v1/titles/"+titleID+"/topics", map[string]string{"If-None-Match": etag})
	if second.Code != http.StatusNotModified || second.Body.Len() != 0 {
		t.Fatalf("expected 304, got %d %q", second.Code, second.Body.String())
	}
	ims := first.Header().Get("Last-Modified")
	third := get(t, mux, "/api/nx/v1/titles/"+titleID+"/topics", map[string]string{"If-Modified-Since": ims})
	if third.Code != http.StatusNotModified {
		t.Fatalf("expected 304 via If-Modified-Since, got %d", third.Code)
	}
}

func TestListAndDatasetServing(t *testing.T) {
	src := &fakeSource{
		titles: []string{titleID},
		ds: &content.Dataset{
			ID: "d1", TitleID: titleID,
			ValidFrom: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			Sealed:    true,
			Files: []content.DatasetFile{
				{Path: "event/season.bin", Key: "abc123.bin", Size: 5, SHA256: "deadbeef"},
			},
		},
		objs: map[string][]byte{"abc123.bin": []byte("hello")},
	}
	mux := newTestService(src)

	rec := get(t, mux, "/v1/list")
	if !strings.Contains(rec.Body.String(), titleID) {
		t.Fatalf("list: %s", rec.Body.String())
	}

	rec = get(t, mux, "/api/nx/v1/titles/"+titleID+"/manifest")
	if !strings.Contains(rec.Body.String(), `"event/season.bin"`) {
		t.Fatalf("manifest: %s", rec.Body.String())
	}

	rec = get(t, mux, "/"+titleID+"/event/season.bin")
	if rec.Code != 200 || rec.Body.String() != "hello" {
		t.Fatalf("file: %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Content-SHA256") != "deadbeef" {
		t.Fatal("missing sha256 header")
	}

	// Path cleaning is done by net/http; validate a traversal segment
	// directly at the wire boundary too.
	if content.ValidateFilePath("../etc/passwd") == nil {
		t.Fatal("traversal path must be rejected")
	}
	if rec := get(t, mux, "/"+titleID+"/nope.bin"); rec.Code != 404 {
		t.Fatalf("unknown file: %d", rec.Code)
	}
	if rec := get(t, mux, "/0100deadbeef0000/event/season.bin"); rec.Code != 404 {
		t.Fatalf("unknown title: %d", rec.Code)
	}
}

func TestEmulatorDelivery(t *testing.T) {
	src := &fakeSource{
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
	rec = get(t, mux, "/api/emulator/v1/bcat/titles/"+titleID+"/files/data/event.bin")
	if rec.Code != 200 || rec.Body.String() != "abc" {
		t.Fatalf("emu file: %d %q", rec.Code, rec.Body.String())
	}
}

func TestRendererContract(t *testing.T) {
	// NB-08: the Switch renderer is a render.Renderer (audience in,
	// console-format out).
	src := &fakeSource{articles: []content.Article{testArticle("global", content.Audience{},
		content.ArticleLocale{Language: "en-US", Title: "t", Body: "b"})}}
	svc := NewService(src, "https://x.example", nil)
	var r render.Renderer = svc.Renderer()
	p, err := r.Render(context.Background(), content.AudienceQuery{Platform: "switch", TitleID: titleID, Regions: []string{"US"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Name() != "switch-bcat" || p.ContentType != "application/json" || !bytes.Contains(p.Body, []byte(`"t"`)) {
		t.Fatalf("payload: %+v %s", p, p.Body)
	}
	if p.ETag == "" {
		t.Fatal("payload must carry an ETag")
	}
}
