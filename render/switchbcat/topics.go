package switchbcat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/openpak/news/internal/content"
)

// topicsJSON is the Switch News topics wire shape (NB-01). NOTE (NB-00
// spike): the exact field names the console accepts are being confirmed on
// hardware; this file is the single place to adjust them. Method: serve one
// topic through the adapter, read the console's behaviour and the adapter's
// debug log, then correct the shape here.
type topicsResponse struct {
	Format int     `json:"format"`
	Topics []topic `json:"topics"`
}

type topic struct {
	ID        string      `json:"id"`
	TitleID   string      `json:"title_id"`
	Language  string      `json:"language"`
	Countries []string    `json:"countries"`
	Datetime  string      `json:"datetime"`
	Headline  string      `json:"headline"`
	Paraphase string      `json:"paraphrase"`
	Body      string      `json:"body"`
	Image     *topicImage `json:"image,omitempty"`
}

type topicImage struct {
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	MD5    string `json:"md5"`
	SHA256 string `json:"sha256"`
}

func topicsJSON(articles []content.Article, q content.AudienceQuery, baseURL string) ([]byte, error) {
	resp := topicsResponse{Format: 1, Topics: []topic{}}
	now := time.Now()
	for i := range articles {
		a := &articles[i]
		if a.PublishedAt == nil || a.PublishedAt.After(now) ||
			(a.ExpiresAt != nil && !a.ExpiresAt.After(now)) {
			continue
		}
		if len(a.Audience.Languages) > 0 && q.Language != "" && !containsFold(a.Audience.Languages, q.Language) {
			continue
		}
		loc := a.Locale(q.Language)
		if loc == nil {
			continue
		}
		t := topic{
			ID:        TopicID(a.ID, loc.Language),
			TitleID:   q.TitleID,
			Language:  loc.Language,
			Countries: a.Audience.Regions,
			Datetime:  a.PublishedAt.UTC().Format("2006-01-02T15:04:05"),
			Headline:  loc.Title,
			Paraphase: paraphrase(loc.Body),
			Body:      loc.Body,
		}
		if t.Countries == nil {
			t.Countries = q.Regions
		}
		if img := imageFor(a, loc.Language); img != nil {
			t.Image = &topicImage{
				URL:    baseURL + "/images/" + img.Key,
				Size:   img.Size,
				MD5:    img.MD5,
				SHA256: img.SHA256,
			}
		}
		resp.Topics = append(resp.Topics, t)
	}
	return json.Marshal(resp)
}

// TopicID derives the stable public topic id: sha256(articleID + language),
// 16 lowercase hex chars.
func TopicID(articleID, language string) string {
	h := sha256.Sum256([]byte(articleID + "|" + language))
	return hex.EncodeToString(h[:8])
}

func imageFor(a *content.Article, language string) *content.ArticleImage {
	for i := range a.Images {
		if a.Images[i].Language == "" || strings.EqualFold(a.Images[i].Language, language) {
			return &a.Images[i]
		}
	}
	return nil
}

// paraphrase is the first paragraph, cut to ~160 chars at a word boundary.
func paraphrase(body string) string {
	p := body
	if i := strings.Index(p, "\n\n"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimSpace(strings.ReplaceAll(p, "\n", " "))
	if len(p) <= 160 {
		return p
	}
	cut := strings.LastIndex(p[:160], " ")
	if cut < 80 {
		cut = 160
	}
	return p[:cut] + "…"
}

func containsFold(list []string, v string) bool {
	for _, s := range list {
		if strings.EqualFold(s, v) {
			return true
		}
	}
	return false
}
