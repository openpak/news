// Package content is the platform-neutral heart of the news service:
// article model, audience targeting, the read API and the authoring gRPC
// live here (PRD NB-03, NB-07, NB-08). Console channels are renderers and
// read from this package only.
package content

import (
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrValidation = errors.New("validation")
)

var (
	reSlug      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)
	reTitleID   = regexp.MustCompile(`^[0-9a-z_-]{1,64}$`) // per-platform title ids (article targeting)
	reSwitchTID = regexp.MustCompile(`^[0-9a-f]{16}$`)     // Switch title ids (datasets)
	reRegion    = regexp.MustCompile(`^[A-Za-z]{2}$`)
	reLang      = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
	reToken     = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)
)

// TokenAll is the explicit "all" value for every audience dimension; it is
// normalized to an empty slice at write time (empty = all).
const TokenAll = "all"

// Audience is article-side targeting. An empty dimension means "all"; the
// explicit value "all" is accepted and normalized to empty.
type Audience struct {
	Platforms []string // switch, wiiu, 3ds, web, android, ios, ps4, xbox, ...
	TitleIDs  []string // per-platform title ids
	Regions   []string // ISO 3166-1 alpha-2
	Languages []string // BCP 47
}

// AudienceQuery is one channel's request: who is asking. Regions carries
// the console's c[] country list (any-of).
type AudienceQuery struct {
	Platform string
	TitleID  string
	Regions  []string
	Language string
}

// Article is a news item written once, delivered everywhere (PRD §1).
type Article struct {
	ID          string
	Slug        string
	Audience    Audience
	PublishedAt *time.Time // nil = draft, never served
	ExpiresAt   *time.Time // nil = never expires
	Locales     []ArticleLocale
	Images      []ArticleImage
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ArticleLocale struct {
	Language string
	Title    string
	Body     string // plain text; \n\n separates paragraphs
}

type ArticleImage struct {
	ID       string
	Language string // "" = all languages
	FileName string
	Key      string // object store key
	URL      string // filled by the service layer (PublicBaseURL)
	Size     int64
	SHA256   string
	MD5      string
}

// Matches reports whether the article is visible to the query audience.
func (a *Article) Matches(q AudienceQuery) bool {
	if q.Platform != "" && !matchesAll(a.Audience.Platforms, q.Platform) {
		return false
	}
	if q.TitleID != "" && !matchesAll(a.Audience.TitleIDs, q.TitleID) {
		return false
	}
	// Regions: any requested region must hit (article "all" always hits).
	if len(q.Regions) > 0 {
		hit := len(a.Audience.Regions) == 0
		for _, r := range q.Regions {
			if matchesAll(a.Audience.Regions, r) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

func matchesAll(targets []string, v string) bool {
	if len(targets) == 0 {
		return true // "all"
	}
	for _, t := range targets {
		if strings.EqualFold(t, v) {
			return true
		}
	}
	return false
}

// Locale returns the best locale for a language: exact match first, then
// any (articles are written once; a missing locale is not a filter —
// language targeting on Audience decides visibility).
func (a *Article) Locale(language string) *ArticleLocale {
	if len(a.Locales) == 0 {
		return nil
	}
	if language != "" {
		for i := range a.Locales {
			if strings.EqualFold(a.Locales[i].Language, language) {
				return &a.Locales[i]
			}
		}
	}
	return &a.Locales[0]
}

// Dataset is a per-title Switch BCAT dataset (Switch-only by nature,
// PRD §1). Only sealed datasets are served; the current one is the most
// recently created sealed dataset whose validity window covers now.
type Dataset struct {
	ID         string
	TitleID    string
	ValidFrom  time.Time
	ValidUntil *time.Time
	Sealed     bool
	Files      []DatasetFile
}

type DatasetFile struct {
	Path   string
	Key    string // object store key
	Size   int64
	SHA256 string
}

// Reader is the read surface: the read API, every renderer and the
// emulator delivery read through it and nothing else.
type Reader interface {
	// Articles returns published articles visible to the audience, with
	// all locales and images, plus max(updated_at) for cache validators.
	Articles(ctx context.Context, q AudienceQuery) ([]Article, time.Time, error)
	// Titles lists Switch title ids that currently have a served dataset.
	Titles(ctx context.Context) ([]string, time.Time, error)
	// CurrentDataset returns the dataset served for a title now.
	CurrentDataset(ctx context.Context, titleID string) (*Dataset, time.Time, error)
	// ImageByKey looks up an image by object key (URL + hashes).
	ImageByKey(ctx context.Context, key string) (*ArticleImage, time.Time, error)
	// OpenObject opens a content-addressed object.
	OpenObject(ctx context.Context, key string) (io.ReadSeekCloser, int64, time.Time, error)
}

// Writer is the authoring surface the gRPC service needs.
type Writer interface {
	CreateArticle(ctx context.Context, a *Article) error
	UpdateArticle(ctx context.Context, id string, mutate func(*Article) error) error
	SetPublication(ctx context.Context, id string, published, expires *time.Time) error
	Article(ctx context.Context, id string) (*Article, error)
	ArticleBySlug(ctx context.Context, slug string) (*Article, error)
	ListArticles(ctx context.Context, includeUnpublished bool, platform, titleID string) ([]*Article, error)
	AttachImage(ctx context.Context, articleID string, img *ArticleImage) error

	CreateDataset(ctx context.Context, d *Dataset) error
	Dataset(ctx context.Context, id string) (*Dataset, error)
	Datasets(ctx context.Context, titleID string) ([]*Dataset, error)
	PutDatasetFile(ctx context.Context, datasetID string, f DatasetFile) error
	SealDataset(ctx context.Context, id string) error
}

// ---------- validation ----------

func ValidateSlug(s string) error {
	if !reSlug.MatchString(s) {
		return errVal("slug must be lowercase letters/digits/hyphen, <= 80 chars")
	}
	return nil
}

func ValidateTitleID(s string) error {
	if !reTitleID.MatchString(s) {
		return errVal("invalid title id: " + s)
	}
	return nil
}

// ValidateSwitchTitleID is the Switch-specific form for BCAT datasets:
// 16 lowercase hex chars (a Switch title id).
func ValidateSwitchTitleID(s string) error {
	if !reSwitchTID.MatchString(s) {
		return errVal("switch title id must be 16 lowercase hex chars")
	}
	return nil
}

func ValidatePlatform(p string) error {
	if p == TokenAll || !reToken.MatchString(p) {
		return errVal("platform must be a short lowercase token, and \"all\" is not stored as one: " + p)
	}
	return nil
}

func ValidateRegions(rs []string) ([]string, error) {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		if !reRegion.MatchString(r) {
			return nil, errVal("region must be a 2-letter code: " + r)
		}
		out = append(out, strings.ToUpper(r))
	}
	return out, nil
}

func ValidateLanguage(l string) error {
	if !reLang.MatchString(l) {
		return errVal("invalid language tag: " + l)
	}
	return nil
}

// ValidateFilePath rejects unsafe dataset destination paths.
func ValidateFilePath(p string) error {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return errVal("dataset path must be relative")
	}
	if strings.Contains(p, "\x00") {
		return errVal("dataset path has NUL")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return errVal("dataset path has an empty/dot segment")
		}
	}
	if len(p) > 512 {
		return errVal("dataset path too long")
	}
	return nil
}

type validationError string

func (e validationError) Error() string { return string(e) }

func errVal(msg string) error { return errors.Join(ErrValidation, validationError(msg)) }
