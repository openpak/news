package content

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ReadAPI is the platform-neutral read endpoint (PRD NB-07): the website,
// companion apps and title servers list and fetch articles for an audience
// with ETag/Last-Modified. Renderers read the same underlying Reader.
type ReadAPI struct {
	Reader Reader
	// PublicBaseURL prefixes image URLs so API consumers can render them.
	PublicBaseURL string
}

type readArticle struct {
	ID          string       `json:"id"`
	Slug        string       `json:"slug"`
	Audience    AudienceJSON `json:"audience"`
	PublishedAt string       `json:"published_at,omitempty"`
	ExpiresAt   string       `json:"expires_at,omitempty"`
	Locales     []readLocale `json:"locales"`
	Images      []readImage  `json:"images,omitempty"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
}

type AudienceJSON struct {
	Platforms []string `json:"platforms"` // empty = all
	TitleIDs  []string `json:"title_ids"`
	Regions   []string `json:"regions"`
	Languages []string `json:"languages"`
}

type readLocale struct {
	Language string `json:"language"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

type readImage struct {
	Language string `json:"language,omitempty"`
	FileName string `json:"file_name"`
	URL      string `json:"url"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
	MD5      string `json:"md5"`
}

func (h *ReadAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/articles" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	qp := r.URL.Query()
	q := AudienceQuery{
		Platform: qp.Get("platform"),
		TitleID:  qp.Get("title"),
		Regions:  qp["region"],
		Language: qp.Get("language"),
	}
	// Same query, applied as a filter list in SQL. Platform/title missing
	// means the caller wants everything visible on that axis.
	articles, mod, err := h.Reader.Articles(r.Context(), q)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	out := make([]readArticle, 0, len(articles))
	for _, a := range articles {
		images := make([]readImage, 0, len(a.Images))
		for _, img := range a.Images {
			images = append(images, readImage{
				Language: img.Language,
				FileName: img.FileName,
				URL:      h.PublicBaseURL + "/images/" + url.PathEscape(img.Key),
				Size:     img.Size,
				SHA256:   img.SHA256,
				MD5:      img.MD5,
			})
		}
		out = append(out, readArticle{
			ID:          a.ID,
			Slug:        a.Slug,
			Audience:    AudienceJSON{a.Audience.Platforms, a.Audience.TitleIDs, a.Audience.Regions, a.Audience.Languages},
			PublishedAt: rfc3339(a.PublishedAt),
			ExpiresAt:   rfc3339(a.ExpiresAt),
			Locales:     localesOf(&a),
			Images:      images,
			CreatedAt:   a.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   a.UpdatedAt.Format(time.RFC3339),
		})
	}
	body, _ := json.MarshalIndent(struct {
		Articles []readArticle `json:"articles"`
	}{out}, "", "  ")

	etag := strongETag(body)
	w.Header().Set("Content-Type", "application/json")
	if !serveConditional(w, r, etag, mod, int64(len(body))) {
		return
	}
	if r.Method == http.MethodGet {
		w.Write(body)
	}
}

func localesOf(a *Article) []readLocale {
	out := make([]readLocale, 0, len(a.Locales))
	for _, l := range a.Locales {
		out = append(out, readLocale{l.Language, l.Title, l.Body})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Language < out[j].Language })
	return out
}

func rfc3339(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// strongETag is a strong validator from the response body.
func strongETag(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:16])
}

// serveConditional writes caching headers and reports whether the body
// should still be written (false = 304 already written). PRD NB-06.
func serveConditional(w http.ResponseWriter, r *http.Request, etag string, mod time.Time, size int64) bool {
	etagQ := `"` + etag + `"`
	w.Header().Set("ETag", etagQ)
	w.Header().Set("Cache-Control", "public, max-age=300")
	if !mod.IsZero() {
		w.Header().Set("Last-Modified", mod.UTC().Format(http.TimeFormat))
	}
	if size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	if r.Header.Get("If-None-Match") != "" {
		if etagMatch(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return false
		}
	} else if ims := r.Header.Get("If-Modified-Since"); ims != "" && !mod.IsZero() {
		if t, err := http.ParseTime(ims); err == nil && !mod.Truncate(time.Second).After(t) {
			w.WriteHeader(http.StatusNotModified)
			return false
		}
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return false
	}
	return true
}

func etagMatch(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "W/")
		part = strings.Trim(part, `"`)
		if part == "*" || part == etag {
			return true
		}
	}
	return false
}
