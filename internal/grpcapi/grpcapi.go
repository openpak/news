// Package grpcapi is the authoring gRPC (PRD NB-2, NB-4): the website's
// role-gated pages and the owner's tooling create articles and BCAT
// datasets through it. Operators only — every call must carry the shared
// operator key; there is no anonymous call.
package grpcapi

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
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

type Service struct {
	newsv1.UnimplementedNewsServer
	writer content.Writer
	reader content.Reader
	objs   objects.Store
	cfg    *config.Config
	log    *slog.Logger
}

func New(writer content.Writer, reader content.Reader, objs objects.Store, cfg *config.Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{writer: writer, reader: reader, objs: objs, cfg: cfg, log: log}
}

// OperatorKeyInterceptor enforces the shared operator key on every RPC.
func OperatorKeyInterceptor(key string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !authorized(ctx, key) {
			return nil, status.Error(codes.PermissionDenied, "operator key required")
		}
		return handler(ctx, req)
	}
}

func authorized(ctx context.Context, key string) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	for _, k := range []string{"authorization", "x-operator-key"} {
		for _, v := range md.Get(k) {
			v = strings.TrimSpace(v)
			if v == key || strings.EqualFold(v, "bearer "+key) {
				return true
			}
		}
	}
	return false
}

// ---------- articles ----------

func (s *Service) CreateArticle(ctx context.Context, req *newsv1.CreateArticleRequest) (*newsv1.Article, error) {
	a := &content.Article{
		Slug:     req.GetSlug(),
		Audience: audienceFrom(req.GetAudience()),
		Locales:  localesFrom(req.GetLocales()),
	}
	if err := validateNew(a); err != nil {
		return nil, invalid(err)
	}
	if err := s.writer.CreateArticle(ctx, a); err != nil {
		return nil, s.storeErr(err)
	}
	s.log.Info("article created", "id", a.ID, "slug", a.Slug)
	return articleTo(a), nil
}

func (s *Service) UpdateArticle(ctx context.Context, req *newsv1.UpdateArticleRequest) (*newsv1.Article, error) {
	in := req.GetAudience()
	// Explicit replacement: a request audience replaces every dimension it
	// lists; the "all" token normalizes to empty (all).
	platforms := normalizeAll(in.GetPlatforms())
	titleIDs := normalizeAll(in.GetTitleIds())
	regions, err := content.ValidateRegions(in.GetRegions())
	if err != nil {
		return nil, invalid(err)
	}
	for _, r := range regions {
		if r == strings.ToUpper(content.TokenAll) {
			regions = nil
			break
		}
	}
	languages := normalizeAll(in.GetLanguages())
	for _, p := range platforms {
		if err := content.ValidatePlatform(p); err != nil {
			return nil, invalid(err)
		}
	}
	for _, t := range titleIDs {
		if err := content.ValidateTitleID(t); err != nil {
			return nil, invalid(err)
		}
	}
	for _, l := range languages {
		if err := content.ValidateLanguage(l); err != nil {
			return nil, invalid(err)
		}
	}

	if err := s.writer.UpdateArticle(ctx, req.GetId(), func(a *content.Article) error {
		if in != nil {
			a.Audience = content.Audience{Platforms: platforms, TitleIDs: titleIDs, Regions: regions, Languages: languages}
		}
		for _, l := range localesFrom(req.GetLocales()) {
			replaced := false
			for i := range a.Locales {
				if strings.EqualFold(a.Locales[i].Language, l.Language) {
					a.Locales[i] = l
					replaced = true
					break
				}
			}
			if !replaced {
				a.Locales = append(a.Locales, l)
			}
		}
		sort.Slice(a.Locales, func(i, j int) bool { return a.Locales[i].Language < a.Locales[j].Language })
		return nil
	}); err != nil {
		return nil, s.storeErr(err)
	}
	a, err := s.writer.Article(ctx, req.GetId())
	if err != nil {
		return nil, s.storeErr(err)
	}
	return articleTo(a), nil
}

func (s *Service) PublishArticle(ctx context.Context, req *newsv1.PublishArticleRequest) (*newsv1.Article, error) {
	published := time.Now().UTC()
	if req.GetPublishedAt() != "" {
		t, err := time.Parse(time.RFC3339, req.GetPublishedAt())
		if err != nil {
			return nil, invalid(fmt.Errorf("published_at: %w", err))
		}
		published = t
	}
	var expires *time.Time
	if req.GetExpiresAt() != "" {
		t, err := time.Parse(time.RFC3339, req.GetExpiresAt())
		if err != nil {
			return nil, invalid(fmt.Errorf("expires_at: %w", err))
		}
		expires = &t
	}
	if err := s.writer.SetPublication(ctx, req.GetId(), &published, expires); err != nil {
		return nil, s.storeErr(err)
	}
	a, err := s.writer.Article(ctx, req.GetId())
	if err != nil {
		return nil, s.storeErr(err)
	}
	s.log.Info("article published", "id", a.ID, "published_at", published)
	return articleTo(a), nil
}

func (s *Service) UnpublishArticle(ctx context.Context, ref *newsv1.ArticleRef) (*newsv1.Article, error) {
	if err := s.writer.SetPublication(ctx, ref.GetId(), nil, nil); err != nil {
		return nil, s.storeErr(err)
	}
	a, err := s.writer.Article(ctx, ref.GetId())
	if err != nil {
		return nil, s.storeErr(err)
	}
	return articleTo(a), nil
}

func (s *Service) GetArticle(ctx context.Context, ref *newsv1.ArticleRef) (*newsv1.Article, error) {
	a, err := s.articleByRef(ctx, ref)
	if err != nil {
		return nil, s.storeErr(err)
	}
	return articleTo(a), nil
}

func (s *Service) ListArticles(ctx context.Context, req *newsv1.ListArticlesRequest) (*newsv1.ListArticlesResponse, error) {
	articles, err := s.writer.ListArticles(ctx, req.GetIncludeUnpublished(), req.GetPlatform(), req.GetTitleId())
	if err != nil {
		return nil, s.storeErr(err)
	}
	out := &newsv1.ListArticlesResponse{}
	for _, a := range articles {
		out.Articles = append(out.Articles, articleTo(a))
	}
	return out, nil
}

func (s *Service) UploadImage(ctx context.Context, req *newsv1.UploadImageRequest) (*newsv1.ArticleImage, error) {
	if err := config.ValidateImageType(req.GetFileName()); err != nil {
		return nil, invalid(err)
	}
	if int64(len(req.GetContent())) > s.cfg.MaxImageSize {
		return nil, invalid(fmt.Errorf("image larger than %d bytes", s.cfg.MaxImageSize))
	}
	if req.GetLanguage() != "" {
		if err := content.ValidateLanguage(req.GetLanguage()); err != nil {
			return nil, invalid(err)
		}
	}
	info, err := s.objs.Put(ctx, &bytesReader{b: req.GetContent(), ext: extOf(req.GetFileName())})
	if err != nil {
		return nil, status.Error(codes.Internal, "object store failed")
	}
	sum := md5.Sum(req.GetContent())
	img := &content.ArticleImage{
		Language: req.GetLanguage(),
		FileName: req.GetFileName(),
		Key:      info.Key,
		URL:      s.cfg.PublicBaseURL + "/images/" + info.Key,
		Size:     info.Size,
		SHA256:   info.SHA256,
		MD5:      hex.EncodeToString(sum[:]),
	}
	if err := s.writer.AttachImage(ctx, req.GetArticleId(), img); err != nil {
		return nil, s.storeErr(err)
	}
	return imageTo(img), nil
}

// RenderPreview renders the article as plain HTML for the authoring pages
// (NB-04). It never touches the console format.
func (s *Service) RenderPreview(ctx context.Context, ref *newsv1.ArticleRef) (*newsv1.RenderPreviewResponse, error) {
	a, err := s.articleByRef(ctx, ref)
	if err != nil {
		return nil, s.storeErr(err)
	}
	return &newsv1.RenderPreviewResponse{Html: previewHTML(a)}, nil
}

// ---------- datasets (Switch-only, PRD §1) ----------

func (s *Service) CreateDataset(ctx context.Context, req *newsv1.CreateDatasetRequest) (*newsv1.Dataset, error) {
	if err := content.ValidateSwitchTitleID(req.GetTitleId()); err != nil {
		return nil, invalid(err)
	}
	d := &content.Dataset{TitleID: req.GetTitleId(), ValidFrom: time.Now().UTC()}
	if req.GetValidFrom() != "" {
		t, err := time.Parse(time.RFC3339, req.GetValidFrom())
		if err != nil {
			return nil, invalid(fmt.Errorf("valid_from: %w", err))
		}
		d.ValidFrom = t
	}
	if req.GetValidUntil() != "" {
		t, err := time.Parse(time.RFC3339, req.GetValidUntil())
		if err != nil {
			return nil, invalid(fmt.Errorf("valid_until: %w", err))
		}
		if !t.After(d.ValidFrom) {
			return nil, invalid(errors.New("valid_until must be after valid_from"))
		}
		d.ValidUntil = &t
	}
	if err := s.writer.CreateDataset(ctx, d); err != nil {
		return nil, s.storeErr(err)
	}
	return datasetTo(d), nil
}

func (s *Service) PutDatasetFile(ctx context.Context, req *newsv1.PutDatasetFileRequest) (*newsv1.DatasetFile, error) {
	if err := content.ValidateFilePath(req.GetPath()); err != nil {
		return nil, invalid(err)
	}
	if int64(len(req.GetContent())) > s.cfg.MaxFileSize {
		return nil, invalid(fmt.Errorf("file larger than %d bytes", s.cfg.MaxFileSize))
	}
	d, err := s.writer.Dataset(ctx, req.GetDatasetId())
	if err != nil {
		return nil, s.storeErr(err)
	}
	if d.Sealed {
		return nil, status.Error(codes.FailedPrecondition, "dataset is sealed")
	}
	info, err := s.objs.Put(ctx, &bytesReader{b: req.GetContent()})
	if err != nil {
		return nil, status.Error(codes.Internal, "object store failed")
	}
	f := content.DatasetFile{Path: req.GetPath(), Key: info.Key, Size: info.Size, SHA256: info.SHA256}
	if err := s.writer.PutDatasetFile(ctx, req.GetDatasetId(), f); err != nil {
		return nil, s.storeErr(err)
	}
	s.log.Info("dataset file added", "dataset", req.GetDatasetId(), "path", f.Path, "size", f.Size)
	return fileTo(&f), nil
}

func (s *Service) SealDataset(ctx context.Context, req *newsv1.SealDatasetRequest) (*newsv1.Dataset, error) {
	if err := s.writer.SealDataset(ctx, req.GetDatasetId()); err != nil {
		return nil, s.storeErr(err)
	}
	d, err := s.writer.Dataset(ctx, req.GetDatasetId())
	if err != nil {
		return nil, s.storeErr(err)
	}
	s.log.Info("dataset sealed", "dataset", d.ID, "title", d.TitleID, "files", len(d.Files))
	return datasetTo(d), nil
}

func (s *Service) ListDatasets(ctx context.Context, req *newsv1.ListDatasetsRequest) (*newsv1.ListDatasetsResponse, error) {
	datasets, err := s.writer.Datasets(ctx, req.GetTitleId())
	if err != nil {
		return nil, s.storeErr(err)
	}
	out := &newsv1.ListDatasetsResponse{}
	for _, d := range datasets {
		out.Datasets = append(out.Datasets, datasetTo(d))
	}
	return out, nil
}

// ---------- mapping + helpers ----------

func (s *Service) articleByRef(ctx context.Context, ref *newsv1.ArticleRef) (*content.Article, error) {
	if ref.GetId() != "" {
		return s.writer.Article(ctx, ref.GetId())
	}
	if ref.GetSlug() != "" {
		return s.writer.ArticleBySlug(ctx, ref.GetSlug())
	}
	return nil, invalid(errors.New("id or slug required"))
}

func (s *Service) storeErr(err error) error {
	switch {
	case errors.Is(err, content.ErrNotFound):
		return status.Error(codes.NotFound, "not found")
	case errors.Is(err, content.ErrConflict):
		return status.Error(codes.FailedPrecondition, "conflict")
	case errors.Is(err, content.ErrValidation):
		return invalid(err)
	}
	s.log.Error("store error", "err", err)
	return status.Error(codes.Internal, "internal error")
}

func invalid(err error) error {
	return status.Error(codes.InvalidArgument, err.Error())
}

func validateNew(a *content.Article) error {
	if err := content.ValidateSlug(a.Slug); err != nil {
		return err
	}
	for _, p := range a.Audience.Platforms {
		if err := content.ValidatePlatform(p); err != nil {
			return err
		}
	}
	for _, t := range a.Audience.TitleIDs {
		if err := content.ValidateTitleID(t); err != nil {
			return err
		}
	}
	for _, r := range a.Audience.Regions {
		if _, err := content.ValidateRegions([]string{r}); err != nil {
			return err
		}
	}
	for _, l := range a.Audience.Languages {
		if err := content.ValidateLanguage(l); err != nil {
			return err
		}
	}
	if len(a.Locales) == 0 {
		return errors.New("at least one locale is required")
	}
	for _, l := range a.Locales {
		if err := content.ValidateLanguage(l.Language); err != nil {
			return err
		}
		if strings.TrimSpace(l.Title) == "" || strings.TrimSpace(l.Body) == "" {
			return errors.New("locale title and body are required")
		}
	}
	return nil
}

// normalizeAll: the explicit "all" token wins and collapses to nil;
// otherwise the list passes through.
func normalizeAll(list []string) []string {
	for _, v := range list {
		if v == content.TokenAll {
			return nil
		}
	}
	return list
}

func audienceFrom(in *newsv1.Audience) content.Audience {
	if in == nil {
		return content.Audience{}
	}
	norm := func(list []string) []string {
		for _, v := range list {
			if v == content.TokenAll {
				return nil
			}
		}
		return list
	}
	regions, err := content.ValidateRegions(in.GetRegions())
	if err != nil {
		regions = nil
	}
	return content.Audience{
		Platforms: norm(in.GetPlatforms()),
		TitleIDs:  norm(in.GetTitleIds()),
		Regions:   regions,
		Languages: norm(in.GetLanguages()),
	}
}

func localesFrom(in []*newsv1.ArticleLocale) []content.ArticleLocale {
	out := make([]content.ArticleLocale, 0, len(in))
	for _, l := range in {
		out = append(out, content.ArticleLocale{Language: l.GetLanguage(), Title: l.GetTitle(), Body: l.GetBody()})
	}
	return out
}

func articleTo(a *content.Article) *newsv1.Article {
	out := &newsv1.Article{
		Id:        a.ID,
		Slug:      a.Slug,
		Audience:  audienceTo(&a.Audience),
		CreatedAt: a.CreatedAt.Format(time.RFC3339),
		UpdatedAt: a.UpdatedAt.Format(time.RFC3339),
	}
	if a.PublishedAt != nil {
		out.PublishedAt = a.PublishedAt.Format(time.RFC3339)
	}
	if a.ExpiresAt != nil {
		out.ExpiresAt = a.ExpiresAt.Format(time.RFC3339)
	}
	for _, l := range a.Locales {
		out.Locales = append(out.Locales, &newsv1.ArticleLocale{Language: l.Language, Title: l.Title, Body: l.Body})
	}
	for i := range a.Images {
		out.Images = append(out.Images, imageTo(&a.Images[i]))
	}
	return out
}

func audienceTo(a *content.Audience) *newsv1.Audience {
	return &newsv1.Audience{
		Platforms: a.Platforms,
		TitleIds:  a.TitleIDs,
		Regions:   a.Regions,
		Languages: a.Languages,
	}
}

func imageTo(img *content.ArticleImage) *newsv1.ArticleImage {
	return &newsv1.ArticleImage{
		Id: img.ID, Language: img.Language, FileName: img.FileName,
		Url: img.URL, Size: img.Size, Sha256: img.SHA256, Md5: img.MD5,
	}
}

func datasetTo(d *content.Dataset) *newsv1.Dataset {
	out := &newsv1.Dataset{
		Id: d.ID, TitleId: d.TitleID,
		ValidFrom: d.ValidFrom.Format(time.RFC3339),
		Sealed:    d.Sealed,
	}
	if d.ValidUntil != nil {
		out.ValidUntil = d.ValidUntil.Format(time.RFC3339)
	}
	for i := range d.Files {
		out.Files = append(out.Files, fileTo(&d.Files[i]))
	}
	return out
}

func fileTo(f *content.DatasetFile) *newsv1.DatasetFile {
	return &newsv1.DatasetFile{Path: f.Path, Size: f.Size, Sha256: f.SHA256}
}

// previewHTML is deliberately plain: title, meta, paragraphs, image.
func previewHTML(a *content.Article) string {
	var b strings.Builder
	loc := a.Locale("")
	if loc == nil {
		return "<p>(no locale)</p>"
	}
	fmt.Fprintf(&b, "<!doctype html><meta charset=\"utf-8\"><title>%s</title>", htmlEsc(loc.Title))
	fmt.Fprintf(&b, "<h1>%s</h1>", htmlEsc(loc.Title))
	fmt.Fprintf(&b, "<p><em>audience: platforms=%s titles=%s regions=%s languages=%s</em></p>",
		htmlEsc(strings.Join(a.Audience.Platforms, ",")),
		htmlEsc(strings.Join(a.Audience.TitleIDs, ",")),
		htmlEsc(strings.Join(a.Audience.Regions, ",")),
		htmlEsc(strings.Join(a.Audience.Languages, ",")))
	for _, img := range a.Images {
		if img.Language == "" || strings.EqualFold(img.Language, loc.Language) {
			fmt.Fprintf(&b, "<p><img src=\"%s\" alt=\"\" style=\"max-width:640px\"></p>", htmlEsc(img.URL))
			break
		}
	}
	for _, p := range strings.Split(loc.Body, "\n\n") {
		fmt.Fprintf(&b, "<p>%s</p>", htmlEsc(strings.ReplaceAll(p, "\n", "<br>")))
	}
	return b.String()
}

func htmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")
	return r.Replace(s)
}

func extOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return strings.ToLower(name[i:])
	}
	return ""
}

// bytesReader feeds the object store; Extension lets FS name keys "<sha256><ext>".
type bytesReader struct {
	b   []byte
	ext string
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}

func (r *bytesReader) Extension() string { return r.ext }
