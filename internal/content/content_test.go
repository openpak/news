package content

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestArticleMatches(t *testing.T) {
	pub := time.Now()
	a := &Article{
		PublishedAt: &pub,
		Audience: Audience{
			Platforms: []string{"switch"},
			TitleIDs:  []string{"0100ed9024eb8000"},
			Regions:   []string{"FR", "GB"},
		},
		Locales: []ArticleLocale{{Language: "en-US", Title: "t", Body: "b"}},
	}
	cases := []struct {
		q    AudienceQuery
		want bool
	}{
		{AudienceQuery{Platform: "switch", TitleID: "0100ed9024eb8000", Regions: []string{"FR"}}, true},
		{AudienceQuery{Platform: "switch", TitleID: "0100ed9024eb8000", Regions: []string{"US"}}, false},
		{AudienceQuery{Platform: "switch", TitleID: "0100000000010000"}, false},
		{AudienceQuery{Platform: "ps4"}, false},
		{AudienceQuery{}, false}, // no platform query = "everything" API call: still must match switch-only? No: platform query empty matches all articles.
	}
	cases[4].want = true
	for i, c := range cases {
		if got := a.Matches(c.q); got != c.want {
			t.Errorf("case %d: matches=%v want %v (%+v)", i, got, c.want, c.q)
		}
	}

	all := &Article{PublishedAt: &pub, Locales: a.Locales} // everything "all"
	if !all.Matches(AudienceQuery{Platform: "switch", TitleID: "x", Regions: []string{"US"}}) {
		t.Error("all-targeting article must match any audience")
	}
}

func TestArticleLocaleFallback(t *testing.T) {
	a := &Article{Locales: []ArticleLocale{
		{Language: "en-US", Title: "en"},
		{Language: "fr-FR", Title: "fr"},
	}}
	if got := a.Locale("fr-FR"); got.Title != "fr" {
		t.Errorf("exact locale: %s", got.Title)
	}
	if got := a.Locale("de-DE"); got.Title != "en" {
		t.Errorf("fallback locale: %s", got.Title)
	}
}

type fakeReader struct {
	articles []Article
	mod      time.Time
}

func (f *fakeReader) Articles(ctx context.Context, q AudienceQuery) ([]Article, time.Time, error) {
	return f.articles, f.mod, nil
}

func (f *fakeReader) Titles(ctx context.Context) ([]string, time.Time, error) {
	return nil, f.mod, nil
}

func (f *fakeReader) CurrentDataset(ctx context.Context, titleID string) (*Dataset, time.Time, error) {
	return nil, time.Time{}, ErrNotFound
}

func (f *fakeReader) ImageByKey(ctx context.Context, key string) (*ArticleImage, time.Time, error) {
	return nil, time.Time{}, ErrNotFound
}

func (f *fakeReader) OpenObject(ctx context.Context, key string) (io.ReadSeekCloser, int64, time.Time, error) {
	return nil, 0, time.Time{}, ErrNotFound
}

func TestReadAPIETagAndFilter(t *testing.T) {
	// NB-07: the read API serves platform-neutral JSON with validators.
	pub := time.Now().UTC().Truncate(time.Second)
	mod := pub
	h := &ReadAPI{Reader: &fakeReader{
		mod: pub,
		articles: []Article{{
			ID: "a1", Slug: "hello",
			Audience:    Audience{Platforms: []string{"switch"}},
			PublishedAt: &pub,
			Locales:     []ArticleLocale{{Language: "en-US", Title: "Hi", Body: "Body"}},
			UpdatedAt:   mod,
			CreatedAt:   mod,
		}},
	}, PublicBaseURL: "https://cdn.example"}

	srv := httptest.NewServer(h)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/v1/articles?platform=switch&title=0100ed9024eb8000&region=FR")
	if err != nil {
		t.Fatal(err)
	}
	body := readAll(t, res)
	res.Body.Close()
	if res.StatusCode != 200 || !bytes.Contains(body, []byte(`"slug": "hello"`)) {
		t.Fatalf("read api: %d %s", res.StatusCode, body)
	}
	if !bytes.Contains(body, []byte(`"published_at"`)) {
		t.Fatal("read api must carry publish date")
	}
	etag := res.Header.Get("ETag")
	if etag == "" || res.Header.Get("Last-Modified") == "" {
		t.Fatal("missing validators (NB-06/NB-07)")
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/articles", nil)
	req.Header.Set("If-None-Match", etag)
	res2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res2.Body.Close()
	if res2.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional: %d", res2.StatusCode)
	}

	// Wrong path 404s.
	res3, err := http.Get(srv.URL + "/v1/other")
	if err != nil {
		t.Fatal(err)
	}
	res3.Body.Close()
	if res3.StatusCode != 404 {
		t.Fatalf("unknown path: %d", res3.StatusCode)
	}
}

func readAll(t *testing.T, r *http.Response) []byte {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
