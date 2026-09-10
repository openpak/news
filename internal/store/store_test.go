package store

// Postgres-backed tests for the content SQL (owner review, NB-00 follow-up).
// Skipped unless NEWS_TEST_DATABASE is set; ports.md 23001 is the
// integration-test Postgres. Local: NEWS_TEST_DATABASE=postgres://news:secret@localhost:23001/news_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/openpak/news/internal/content"
	"github.com/openpak/news/internal/objects"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	url := testingEnv()
	if url == "" {
		t.Skip("NEWS_TEST_DATABASE not set")
	}
	ctx := context.Background()
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	// Deterministic state: wipe content tables, re-apply migrations.
	for _, ddl := range []string{
		`DROP TABLE IF EXISTS dataset_files, datasets, article_images, article_locales, articles, schema_migrations CASCADE`,
	} {
		if _, err := pool.Exec(ctx, ddl); err != nil {
			t.Fatalf("reset: %v", err)
		}
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	objs, err := objects.NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("objects: %v", err)
	}
	return New(pool, objs)
}

func testingEnv() string { return os.Getenv("NEWS_TEST_DATABASE") }

func TestArticleLifecycleAndAudienceSQL(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	pub := time.Now().UTC().Add(-time.Hour)

	mk := func(slug string, aud content.Audience) *content.Article {
		a := &content.Article{
			Slug:        slug,
			Audience:    aud,
			PublishedAt: &pub,
			Locales: []content.ArticleLocale{
				{Language: "en-US", Title: "EN " + slug, Body: "body " + slug},
				{Language: "fr-FR", Title: "FR " + slug, Body: "corps " + slug},
			},
		}
		if err := s.CreateArticle(ctx, a); err != nil {
			t.Fatalf("create %s: %v", slug, err)
		}
		return a
	}

	mk("all-platforms", content.Audience{})
	mk("switch-fr", content.Audience{Platforms: []string{"switch"}, Regions: []string{"FR"}})
	mk("stardew", content.Audience{Platforms: []string{"switch"}, TitleIDs: []string{"0100ed9024eb8000"}})
	mk("ps-only", content.Audience{Platforms: []string{"ps4", "ps5"}})

	// Empty query = everything visible.
	all, _, err := s.Articles(ctx, content.AudienceQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("want 4 articles, got %d", len(all))
	}

	// Switch + Stardew title: stardew plus every "all titles" article
	// (empty title_ids = all titles, so switch-fr and all-platforms match).
	got, _, err := s.Articles(ctx, content.AudienceQuery{Platform: "switch", TitleID: "0100ed9024eb8000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Slug != "stardew" || slugs(got)[2] != "all-platforms" {
		t.Fatalf("title filter: %+v", slugs(got))
	}
	// ps-only never matches a switch query.
	for _, a := range got {
		if a.Slug == "ps-only" {
			t.Fatal("ps-only leaked into switch query")
		}
	}

	// Region any-of: empty article regions = all regions, so stardew
	// rides along; switch-fr only matches FR.
	got, _, err = s.Articles(ctx, content.AudienceQuery{Platform: "switch", Regions: []string{"FR"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("region filter FR: %+v", slugs(got))
	}
	got, _, _ = s.Articles(ctx, content.AudienceQuery{Platform: "switch", Regions: []string{"US"}})
	if len(got) != 2 {
		t.Fatalf("region miss US: %+v", slugs(got))
	}
	for _, a := range got {
		if a.Slug == "switch-fr" {
			t.Fatal("FR-only article matched US query")
		}
	}

	// Language targeting narrows by audience languages.
	stardew, err := s.ArticleBySlug(ctx, "stardew")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateArticle(ctx, stardew.ID, func(a *content.Article) error {
		a.Audience.Languages = []string{"fr-FR"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got, _, _ = s.Articles(ctx, content.AudienceQuery{Platform: "switch", TitleID: "0100ed9024eb8000", Language: "en-US"})
	// stardew drops out (fr-FR only); the "all languages" articles remain.
	if len(got) != 2 {
		t.Fatalf("language filter: %+v", slugs(got))
	}
	for _, a := range got {
		if a.Slug == "stardew" {
			t.Fatal("fr-FR-targeted article matched en-US query")
		}
	}

	// Locales survive the round trip.
	a, err := s.ArticleBySlug(ctx, "all-platforms")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Locales) != 2 || a.Locales[0].Title != "EN all-platforms" {
		t.Fatalf("locales: %+v", a.Locales)
	}

	// Unpublished articles are invisible to readers.
	psOnly, err := s.ArticleBySlug(ctx, "ps-only")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPublication(ctx, psOnly.ID, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, _, _ = s.Articles(ctx, content.AudienceQuery{Platform: "ps4"})
	if len(got) != 1 || got[0].Slug != "all-platforms" {
		t.Fatalf("ps4 query after unpublish: %+v", slugs(got))
	}
}

func TestDatasetLifecycleSQL(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	const title = "0100ed9024eb8000"

	from0 := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	d := &content.Dataset{TitleID: title, ValidFrom: from0}
	if err := s.CreateDataset(ctx, d); err != nil {
		t.Fatal(err)
	}
	// CreateDataset must return the scheduled valid_from, not created_at:
	// the operator is told when the dataset actually goes live.
	if !d.ValidFrom.Equal(from0) {
		t.Fatalf("valid_from round trip: got %s want %s", d.ValidFrom, from0)
	}
	if err := s.SealDataset(ctx, d.ID); !errors.Is(err, content.ErrValidation) {
		t.Fatalf("empty seal: %v", err)
	}
	f := content.DatasetFile{Path: "event/season.bin", Key: "k1", Size: 3, SHA256: "aa"}
	if err := s.PutDatasetFile(ctx, d.ID, f); err != nil {
		t.Fatal(err)
	}
	if err := s.SealDataset(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.PutDatasetFile(ctx, d.ID, content.DatasetFile{Path: "x", Key: "k2"}); !errors.Is(err, content.ErrConflict) {
		t.Fatalf("sealed put: %v", err)
	}

	cur, _, err := s.CurrentDataset(ctx, title)
	if err != nil {
		t.Fatal(err)
	}
	if cur.ID != d.ID || len(cur.Files) != 1 || cur.Files[0].Path != "event/season.bin" {
		t.Fatalf("current dataset: %+v", cur)
	}
	if _, _, err := s.CurrentDataset(ctx, "0100000000010000"); !errors.Is(err, content.ErrNotFound) {
		t.Fatalf("unknown title: %v", err)
	}

	// A newer dataset supersedes the old one.
	d2 := &content.Dataset{TitleID: title, ValidFrom: time.Now().UTC()}
	if err := s.CreateDataset(ctx, d2); err != nil {
		t.Fatal(err)
	}
	if err := s.PutDatasetFile(ctx, d2.ID, content.DatasetFile{Path: "new.bin", Key: "k3"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SealDataset(ctx, d2.ID); err != nil {
		t.Fatal(err)
	}
	cur, _, _ = s.CurrentDataset(ctx, title)
	if cur.ID != d2.ID {
		t.Fatalf("supersede: got %s want %s", cur.ID, d2.ID)
	}

	titles, _, err := s.Titles(ctx)
	if err != nil || len(titles) != 1 || titles[0] != title {
		t.Fatalf("titles: %v %v", titles, err)
	}
}

func TestImageByKeySQL(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a := &content.Article{
		Slug:     "with-img",
		Audience: content.Audience{},
		Locales:  []content.ArticleLocale{{Language: "en-US", Title: "t", Body: "b"}},
	}
	if err := s.CreateArticle(ctx, a); err != nil {
		t.Fatal(err)
	}
	img := &content.ArticleImage{Key: "obj-abc.jpg", FileName: "abc.jpg", Size: 5, SHA256: "s1", MD5: "m1"}
	if err := s.AttachImage(ctx, a.ID, img); err != nil {
		t.Fatal(err)
	}
	if img.ID == "" {
		t.Fatal("image id not returned")
	}
	got, _, err := s.ImageByKey(ctx, "obj-abc.jpg")
	if err != nil || got.FileName != "abc.jpg" || got.Size != 5 {
		t.Fatalf("image by key: %+v %v", got, err)
	}
	if _, _, err := s.ImageByKey(ctx, "nope"); !errors.Is(err, content.ErrNotFound) {
		t.Fatalf("missing image: %v", err)
	}

	// Attaching an image changes the read API payload, so it must move
	// updated_at — that is the Last-Modified every cache validates on.
	after, err := s.Article(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.UpdatedAt.After(a.UpdatedAt) {
		t.Fatalf("updated_at not touched by AttachImage: %s <= %s", after.UpdatedAt, a.UpdatedAt)
	}
}

func slugs(as []content.Article) []string {
	out := make([]string, 0, len(as))
	for _, a := range as {
		out = append(out, a.Slug)
	}
	return out
}
