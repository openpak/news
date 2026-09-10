package grpcapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	newsv1 "github.com/openpak/news/gen/news/v1"
	"github.com/openpak/news/internal/config"
	"github.com/openpak/news/internal/content"
	"github.com/openpak/news/internal/objects"
)

// fakeWriter implements content.Writer in memory.
type fakeWriter struct {
	articles map[string]*content.Article
	datasets map[string]*content.Dataset
	seq      int
}

func newFakeWriter() *fakeWriter {
	return &fakeWriter{articles: map[string]*content.Article{}, datasets: map[string]*content.Dataset{}}
}

func (f *fakeWriter) CreateArticle(ctx context.Context, a *content.Article) error {
	f.seq++
	a.ID = "id-" + string(rune('0'+f.seq))
	cp := *a
	f.articles[a.ID] = &cp
	return nil
}

func (f *fakeWriter) UpdateArticle(ctx context.Context, id string, mutate func(*content.Article) error) error {
	a, ok := f.articles[id]
	if !ok {
		return content.ErrNotFound
	}
	return mutate(a)
}

func (f *fakeWriter) SetPublication(ctx context.Context, id string, published, expires *time.Time) error {
	a, ok := f.articles[id]
	if !ok {
		return content.ErrNotFound
	}
	a.PublishedAt, a.ExpiresAt = published, expires
	return nil
}

func (f *fakeWriter) Article(ctx context.Context, id string) (*content.Article, error) {
	if a, ok := f.articles[id]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, content.ErrNotFound
}

func (f *fakeWriter) ArticleBySlug(ctx context.Context, slug string) (*content.Article, error) {
	for _, a := range f.articles {
		if a.Slug == slug {
			cp := *a
			return &cp, nil
		}
	}
	return nil, content.ErrNotFound
}

func (f *fakeWriter) ListArticles(ctx context.Context, includeUnpublished bool, platform, titleID string) ([]*content.Article, error) {
	var out []*content.Article
	for _, a := range f.articles {
		if !includeUnpublished && a.PublishedAt == nil {
			continue
		}
		cp := *a
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeWriter) AttachImage(ctx context.Context, articleID string, img *content.ArticleImage) error {
	a, ok := f.articles[articleID]
	if !ok {
		return content.ErrNotFound
	}
	f.seq++
	img.ID = "img-" + string(rune('0'+f.seq))
	a.Images = append(a.Images, *img)
	return nil
}

func (f *fakeWriter) CreateDataset(ctx context.Context, d *content.Dataset) error {
	f.seq++
	d.ID = "ds-" + string(rune('0'+f.seq))
	cp := *d
	f.datasets[d.ID] = &cp
	return nil
}

func (f *fakeWriter) Dataset(ctx context.Context, id string) (*content.Dataset, error) {
	if d, ok := f.datasets[id]; ok {
		cp := *d
		return &cp, nil
	}
	return nil, content.ErrNotFound
}

func (f *fakeWriter) Datasets(ctx context.Context, titleID string) ([]*content.Dataset, error) {
	var out []*content.Dataset
	for _, d := range f.datasets {
		cp := *d
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeWriter) PutDatasetFile(ctx context.Context, datasetID string, file content.DatasetFile) error {
	d, ok := f.datasets[datasetID]
	if !ok {
		return content.ErrNotFound
	}
	if d.Sealed {
		return content.ErrConflict
	}
	d.Files = append(d.Files, file)
	return nil
}

func (f *fakeWriter) SealDataset(ctx context.Context, id string) error {
	d, ok := f.datasets[id]
	if !ok {
		return content.ErrNotFound
	}
	if len(d.Files) == 0 {
		return content.ErrValidation
	}
	d.Sealed = true
	return nil
}

// memObjects implements objects.Store in memory.
type memObjects struct{ m map[string]objects.Info }

func newMemObjects() *memObjects { return &memObjects{m: map[string]objects.Info{}} }

func (m *memObjects) Put(ctx context.Context, r io.Reader) (objects.Info, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return objects.Info{}, err
	}
	key := "obj-" + string(rune('a'+len(m.m)))
	info := objects.Info{Key: key, Size: int64(len(b)), SHA256: key, ModTime: time.Now().UTC()}
	m.m[key] = info
	return info, nil
}

func (m *memObjects) Open(ctx context.Context, key string) (io.ReadSeekCloser, objects.Info, error) {
	info, ok := m.m[key]
	if !ok {
		return nil, objects.Info{}, objects.ErrNotFound
	}
	return &nopCloser{bytes.NewReader(nil)}, info, nil
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

func testCfg() *config.Config {
	return &config.Config{
		PublicBaseURL: "https://bcat-data-lp1.cdn.nintendo.net",
		MaxImageSize:  8 << 20,
		MaxFileSize:   64 << 20,
	}
}

func testService() *Service {
	return New(newFakeWriter(), nil, newMemObjects(), testCfg(), nil)
}

const opKey = "0123456789abcdef0123456789abcdef"

func authCtx(key string) context.Context {
	if key == "" {
		return context.Background()
	}
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+key))
}

func TestOperatorKeyInterceptor(t *testing.T) {
	called := false
	handler := func(ctx context.Context, req any) (any, error) { called = true; return nil, nil }
	inter := OperatorKeyInterceptor(opKey)

	if _, err := inter(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/news.v1.News/GetArticle"}, handler); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("anonymous call: %v", err)
	}
	if _, err := inter(authCtx("wrong-wrong-wrong-wrong-1234"), nil, &grpc.UnaryServerInfo{}, handler); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("wrong key: %v", err)
	}
	if _, err := inter(authCtx(opKey), nil, &grpc.UnaryServerInfo{}, handler); err != nil {
		t.Fatalf("good key rejected: %v", err)
	}
	if !called {
		t.Fatal("handler not reached with good key")
	}
}

func TestArticleLifecycle(t *testing.T) {
	s := testService()
	ctx := authCtx(opKey)

	// Validation: bad slug.
	_, err := s.CreateArticle(ctx, &newsv1.CreateArticleRequest{Slug: "Bad Slug!"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad slug: %v", err)
	}

	created, err := s.CreateArticle(ctx, &newsv1.CreateArticleRequest{
		Slug: "openpak-live",
		Audience: &newsv1.Audience{
			Platforms: []string{content.TokenAll},
			Regions:   []string{"FR", "GB"},
		},
		Locales: []*newsv1.ArticleLocale{
			{Language: "en-US", Title: "OpenPak is live", Body: "Hello console"},
			{Language: "fr-FR", Title: "OpenPak est là", Body: "Bonjour"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Audience.Platforms) != 0 || created.Audience.Platforms != nil && created.Audience.Platforms[0] == content.TokenAll {
		t.Fatalf("\"all\" must normalize to empty: %v", created.Audience.Platforms)
	}
	if got := created.Audience.Regions; len(got) != 2 || got[0] != "FR" {
		t.Fatalf("regions: %v", got)
	}

	// Draft is not listed by default.
	listed, err := s.ListArticles(ctx, &newsv1.ListArticlesRequest{})
	if err != nil || len(listed.Articles) != 0 {
		t.Fatalf("draft leaked: %v %v", err, listed)
	}

	pub, err := s.PublishArticle(ctx, &newsv1.PublishArticleRequest{Id: created.Id})
	if err != nil || pub.PublishedAt == "" {
		t.Fatalf("publish: %v %v", err, pub)
	}

	listed, err = s.ListArticles(ctx, &newsv1.ListArticlesRequest{})
	if err != nil || len(listed.Articles) != 1 {
		t.Fatalf("published not listed: %v %v", err, listed)
	}

	// Update replaces audience, upserts locale.
	upd, err := s.UpdateArticle(ctx, &newsv1.UpdateArticleRequest{
		Id:       created.Id,
		Audience: &newsv1.Audience{Platforms: []string{"switch"}, TitleIds: []string{"0100ed9024eb8000"}},
		Locales:  []*newsv1.ArticleLocale{{Language: "en-US", Title: "OpenPak is live!", Body: "Hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(upd.Audience.Platforms) != 1 || upd.Audience.Platforms[0] != "switch" || len(upd.Audience.Regions) != 0 {
		t.Fatalf("audience replace: %+v", upd.Audience)
	}
	if len(upd.Locales) != 2 || upd.Locales[0].Title != "OpenPak is live!" {
		t.Fatalf("locale upsert: %+v", upd.Locales)
	}

	// Unpublish hides again.
	if _, err := s.UnpublishArticle(ctx, &newsv1.ArticleRef{Id: created.Id}); err != nil {
		t.Fatal(err)
	}
	listed, _ = s.ListArticles(ctx, &newsv1.ListArticlesRequest{})
	if len(listed.Articles) != 0 {
		t.Fatal("unpublished article still listed")
	}
}

func TestPreview(t *testing.T) {
	s := testService()
	ctx := authCtx(opKey)
	created, err := s.CreateArticle(ctx, &newsv1.CreateArticleRequest{
		Slug:    "preview-me",
		Locales: []*newsv1.ArticleLocale{{Language: "en-US", Title: "<Title>", Body: "Para one"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	prev, err := s.RenderPreview(ctx, &newsv1.ArticleRef{Slug: "preview-me"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prev.Html, "&lt;Title&gt;") || !strings.Contains(prev.Html, "<p>Para one</p>") {
		t.Fatalf("preview not escaped/paragraphed: %s", prev.Html)
	}
	_ = created
}

func TestUploadImage(t *testing.T) {
	s := testService()
	ctx := authCtx(opKey)
	created, _ := s.CreateArticle(ctx, &newsv1.CreateArticleRequest{
		Slug: "with-img", Locales: []*newsv1.ArticleLocale{{Language: "en-US", Title: "t", Body: "b"}},
	})

	if _, err := s.UploadImage(ctx, &newsv1.UploadImageRequest{ArticleId: created.Id, FileName: "x.gif", Content: []byte("GIF")}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad type accepted: %v", err)
	}

	img, err := s.UploadImage(ctx, &newsv1.UploadImageRequest{
		ArticleId: created.Id, FileName: "art.JPG", Language: "en-US", Content: []byte("jpegdata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(img.Url, testCfg().PublicBaseURL+"/images/") || img.Md5 == "" || len(img.Sha256) != len("obj-a") {
		t.Fatalf("image mapping: %+v", img)
	}
}

func TestDatasetLifecycle(t *testing.T) {
	s := testService()
	ctx := authCtx(opKey)

	if _, err := s.CreateDataset(ctx, &newsv1.CreateDatasetRequest{TitleId: "nothex"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad title id: %v", err)
	}
	ds, err := s.CreateDataset(ctx, &newsv1.CreateDatasetRequest{
		TitleId:   "0100ed9024eb8000",
		ValidFrom: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.PutDatasetFile(ctx, &newsv1.PutDatasetFileRequest{DatasetId: ds.Id, Path: "../evil", Content: []byte("x")}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("traversal accepted: %v", err)
	}
	if _, err := s.PutDatasetFile(ctx, &newsv1.PutDatasetFileRequest{DatasetId: ds.Id, Path: "/abs", Content: []byte("x")}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("absolute path accepted: %v", err)
	}
	if _, err := s.SealDataset(ctx, &newsv1.SealDatasetRequest{DatasetId: ds.Id}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty dataset sealed: %v", err)
	}

	f, err := s.PutDatasetFile(ctx, &newsv1.PutDatasetFileRequest{DatasetId: ds.Id, Path: "event/season.bin", Content: []byte("season")})
	if err != nil || f.Size != 6 {
		t.Fatalf("put file: %v %+v", err, f)
	}
	if _, err := s.SealDataset(ctx, &newsv1.SealDatasetRequest{DatasetId: ds.Id}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutDatasetFile(ctx, &newsv1.PutDatasetFileRequest{DatasetId: ds.Id, Path: "late.bin", Content: []byte("x")}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("sealed dataset accepted a file: %v", err)
	}
}

func TestNotFoundMapping(t *testing.T) {
	s := testService()
	if _, err := s.GetArticle(authCtx(opKey), &newsv1.ArticleRef{Id: "nope"}); status.Code(err) != codes.NotFound {
		t.Fatalf("not found mapping: %v", err)
	}
	if _, err := s.PublishArticle(authCtx(opKey), &newsv1.PublishArticleRequest{Id: "nope"}); status.Code(err) != codes.NotFound {
		t.Fatalf("not found mapping: %v", err)
	}
}

var _ = errors.New

// A malformed audience value must be rejected, never normalized away —
// dropping it turns targeted news into news for everyone.
func TestAudienceNeverFailsOpen(t *testing.T) {
	s := testService()
	ctx := authCtx(opKey)
	base := func(aud *newsv1.Audience) *newsv1.CreateArticleRequest {
		return &newsv1.CreateArticleRequest{
			Slug:     "probe",
			Audience: aud,
			Locales:  []*newsv1.ArticleLocale{{Language: "en-US", Title: "t", Body: "b"}},
		}
	}
	for _, c := range []struct {
		name string
		aud  *newsv1.Audience
	}{
		{"bad region", &newsv1.Audience{Regions: []string{"FRA"}}},
		{"bad platform", &newsv1.Audience{Platforms: []string{"Switch!"}}},
		{"bad language", &newsv1.Audience{Languages: []string{"english"}}},
		{"bad title id", &newsv1.Audience{TitleIds: []string{"NOT/A/TITLE"}}},
	} {
		if _, err := s.CreateArticle(ctx, base(c.aud)); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: got %v, want InvalidArgument (audience would have widened to all)", c.name, err)
		}
	}

	// "all" on any dimension still normalizes to empty (= all), any case.
	a, err := s.CreateArticle(ctx, base(&newsv1.Audience{
		Platforms: []string{"all"}, Regions: []string{"ALL"}, Languages: []string{"All"},
	}))
	if err != nil {
		t.Fatalf("all token rejected: %v", err)
	}
	if len(a.GetAudience().GetPlatforms()) != 0 || len(a.GetAudience().GetRegions()) != 0 ||
		len(a.GetAudience().GetLanguages()) != 0 {
		t.Errorf("all token not normalized: %+v", a.GetAudience())
	}
}
