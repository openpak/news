// SPDX-License-Identifier: AGPL-3.0-only
package newsadmin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openpak/news/internal/newsseed"
)

// apiError carries the HTTP status an operation failed with.
type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string { return e.msg }

func errf(status int, format string, a ...any) error {
	return &apiError{status: status, msg: fmt.Sprintf(format, a...)}
}

var reAdminTopic = regexp.MustCompile(`^openpak_[a-z0-9_]{1,23}$`)

const (
	maxName       = 100
	maxShopQuery  = 1024
	maxAppIDs     = 64
	maxReason     = 200
	maxImageBytes = 16 << 20
	maxImageSide  = 8192
	deletedReason = "deleted from the admin dashboard"
)

// source is one consistent read of the channels directory.
type source struct {
	lineup   *newsseed.Lineup
	retired  *newsseed.Retired
	articles map[string]map[int64]*newsseed.Article
}

func (s *Service) load() (*source, error) {
	l, err := newsseed.LoadLineup(s.src)
	if err != nil {
		return nil, err
	}
	r, err := newsseed.LoadRetired(filepath.Join(s.src, "retired.json"))
	if err != nil {
		return nil, err
	}
	src := &source{lineup: l, retired: r, articles: map[string]map[int64]*newsseed.Article{}}
	entries, err := os.ReadDir(s.src)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "screenshots" || e.Name() == "icons" {
			continue
		}
		topic := e.Name()
		src.articles[topic] = map[int64]*newsseed.Article{}
		files, err := os.ReadDir(filepath.Join(s.src, topic))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			a, err := newsseed.LoadArticle(filepath.Join(s.src, topic, f.Name()))
			if err != nil {
				return nil, err
			}
			src.articles[topic][a.NewsID] = a
		}
	}
	return src, nil
}

func (src *source) live(topic string) *newsseed.Channel {
	for i := range src.lineup.Channels {
		if src.lineup.Channels[i].TopicID == topic {
			return &src.lineup.Channels[i]
		}
	}
	return nil
}

// retiredChannel returns the snapshot and reason of a channel that was
// removed from the lineup (it lives on in its articles' retired entries).
func (src *source) retiredChannel(topic string) (*newsseed.Channel, string) {
	if src.live(topic) != nil {
		return nil, ""
	}
	for i := range src.retired.Retired {
		e := &src.retired.Retired[i]
		if e.TopicID == topic && e.Topic != nil {
			snap := *e.Topic
			snap.TopicID = topic
			return &snap, e.Reason
		}
	}
	return nil, ""
}

func (src *source) retiredEntry(topic string, id int64) *newsseed.RetiredEntry {
	for i := range src.retired.Retired {
		if e := &src.retired.Retired[i]; e.TopicID == topic && e.NewsID == id {
			return e
		}
	}
	return nil
}

func (src *source) usedID(id int64) bool {
	for _, arts := range src.articles {
		if arts[id] != nil {
			return true
		}
	}
	return false
}

// ---------- views ----------

type channelView struct {
	TopicID            string       `json:"topic_id"`
	Name               string       `json:"name"`
	Publisher          string       `json:"publisher"`
	Description        string       `json:"description"`
	Important          bool         `json:"important"`
	Icon               newsseed.Art `json:"icon"`
	HasIconImage       bool         `json:"has_icon_image"`
	ShopQuery          string       `json:"shop_query"`
	ApplicationIDs     []string     `json:"application_ids"`
	TestDistribution   bool         `json:"test_distribution"`
	Retired            bool         `json:"retired"`
	RetireReason       string       `json:"retire_reason"`
	ArticleCount       int          `json:"article_count"`
	ActiveArticleCount int          `json:"active_article_count"`
}

func (s *Service) channelView(src *source, ch *newsseed.Channel, retired bool, reason string) channelView {
	v := channelView{
		TopicID: ch.TopicID, Name: ch.Name, Publisher: ch.Publisher, Description: ch.Description,
		Important: ch.Important, Icon: ch.Icon, ShopQuery: ch.ShopQuery,
		ApplicationIDs: append([]string{}, ch.ApplicationIDs...), TestDistribution: ch.TestDistribution,
		Retired: retired, RetireReason: reason,
		HasIconImage: imageExists(newsseed.IconPath(s.src, ch.TopicID)),
	}
	for id := range src.articles[ch.TopicID] {
		v.ArticleCount++
		if src.retiredEntry(ch.TopicID, id) == nil {
			v.ActiveArticleCount++
		}
	}
	return v
}

type essentialView struct {
	PickupDays    int `json:"pickup_days"`
	PriorityAfter int `json:"priority_after"`
}

type articleView struct {
	TopicID            string         `json:"topic_id"`
	NewsID             int64          `json:"news_id"`
	Title              string         `json:"title"`
	Body               string         `json:"body"`
	Footer             string         `json:"footer"`
	Language           string         `json:"language"`
	SupportedLanguages []string       `json:"supported_languages"`
	LifetimeDays       int            `json:"lifetime_days"`
	PublishedAt        *string        `json:"published_at"`
	ExpireAt           *string        `json:"expire_at"`
	PickupDays         *int           `json:"pickup_days"`
	Priority           int            `json:"priority"`
	EssentialPickup    *essentialView `json:"essential_pickup"`
	Art                newsseed.Art   `json:"art"`
	HasImage           bool           `json:"has_image"`
	More               *newsseed.More `json:"more"`
	Retired            bool           `json:"retired"`
	RetireReason       string         `json:"retire_reason"`
	EverPublished      bool           `json:"ever_published"`
}

func (s *Service) articleView(src *source, topic string, a *newsseed.Article, published map[string]bool) articleView {
	v := articleView{
		TopicID: topic, NewsID: a.NewsID, Title: a.Title, Body: a.Body, Footer: a.Footer,
		Language: a.Language, SupportedLanguages: append([]string{}, a.SupportedLanguages...),
		LifetimeDays: a.LifetimeDays, PickupDays: a.PickupDays, Priority: a.Priority,
		Art: a.Art, More: a.More,
		HasImage:      imageExists(newsseed.ScreenshotPath(s.src, a.NewsID)),
		EverPublished: published[topic+"/"+itoa(a.NewsID)],
	}
	if v.Priority == 0 {
		v.Priority = 100
	}
	if a.PublishedAt != "" {
		p := a.PublishedAt
		v.PublishedAt = &p
		if t, err := time.Parse(time.RFC3339, p); err == nil {
			e := t.Add(time.Duration(a.LifetimeDays) * 24 * time.Hour).UTC().Format(time.RFC3339)
			v.ExpireAt = &e
		}
	}
	if e := a.EssentialPickup; e != nil {
		v.EssentialPickup = &essentialView{PickupDays: e.PickupDays, PriorityAfter: e.PriorityAfter}
		if v.EssentialPickup.PriorityAfter == 0 {
			v.EssentialPickup.PriorityAfter = v.Priority
		}
	}
	if e := src.retiredEntry(topic, a.NewsID); e != nil {
		v.Retired, v.RetireReason = true, e.Reason
	}
	return v
}

// publishedIDs is every "topic/id" a build (or the imported file-based
// lineup) has shipped. Ids are never removed from later manifests once
// shipped, so the newest manifest is enough.
func (s *Service) publishedIDs() map[string]bool {
	out := map[string]bool{}
	for _, id := range s.state.ImportedIDs {
		out[id] = true
	}
	for _, raw := range [][]byte{s.newestManifest(), s.importManifest()} {
		var m newsseed.Manifest
		if raw == nil || json.Unmarshal(raw, &m) != nil {
			continue
		}
		for _, ch := range m.Channels {
			for _, a := range ch.Articles {
				out[ch.TopicID+"/"+itoa(a.NewsID)] = true
			}
		}
		for _, ch := range m.Removed {
			for _, a := range ch.Articles {
				out[ch.TopicID+"/"+itoa(a.NewsID)] = true
			}
		}
	}
	return out
}

// newestManifest is the newest build's manifest: the overwrite baseline.
func (s *Service) newestManifest() []byte {
	if n := len(s.state.Builds); n > 0 {
		raw, err := os.ReadFile(filepath.Join(s.buildDir(s.state.Builds[n-1].Build), "seed", "manifest.json"))
		if err == nil {
			return raw
		}
	}
	return nil
}

func (s *Service) importManifest() []byte {
	raw, err := os.ReadFile(filepath.Join(s.cfg.DataDir, "import-manifest.json"))
	if err != nil {
		return nil
	}
	return raw
}

// ---------- JSON merge ----------

// mergeInto applies a JSON merge patch (absent = keep, null = default) to
// base and decodes the result strictly into out. Keys in ignore are
// read-only view fields and are skipped.
func mergeInto(base any, patch map[string]json.RawMessage, ignore map[string]bool, out any) error {
	raw, err := json.Marshal(base)
	if err != nil {
		return err
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	for k, v := range patch {
		if ignore[k] {
			continue
		}
		if string(bytes.TrimSpace(v)) == "null" {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	raw, err = json.Marshal(m)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return errf(http.StatusBadRequest, "invalid field: %v", strings.TrimPrefix(err.Error(), "json: "))
	}
	return nil
}

var channelReadOnly = map[string]bool{"has_icon_image": true, "article_count": true, "active_article_count": true, "retired": true, "retire_reason": true}

var articleReadOnly = map[string]bool{"has_image": true, "expire_at": true, "ever_published": true, "retired": true, "retire_reason": true, "topic_id": true}

// retireFlag extracts the retired / retire_reason keys of a patch.
func retireFlag(patch map[string]json.RawMessage) (set bool, retired bool, reason string, err error) {
	if raw, ok := patch["retired"]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, &retired); err != nil {
			return false, false, "", errf(http.StatusBadRequest, "retired must be a boolean")
		}
		set = true
	}
	if raw, ok := patch["retire_reason"]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, &reason); err != nil {
			return false, false, "", errf(http.StatusBadRequest, "retire_reason must be a string")
		}
	}
	reason = strings.TrimSpace(reason)
	if set && retired && (reason == "" || len(reason) > maxReason) {
		return false, false, "", errf(http.StatusBadRequest, "retire_reason is required (1..%d bytes) to retire", maxReason)
	}
	return set, retired, reason, nil
}

// onlyKeys reports whether patch has no keys outside allowed and ignored.
func onlyKeys(patch map[string]json.RawMessage, ignore map[string]bool, allowed ...string) bool {
	for k := range patch {
		if ignore[k] {
			continue
		}
		ok := false
		for _, a := range allowed {
			ok = ok || k == a
		}
		if !ok {
			return false
		}
	}
	return true
}

func checkChannel(ch *newsseed.Channel) error {
	if err := ch.Validate(); err != nil {
		return errf(http.StatusBadRequest, "%v", err)
	}
	switch {
	case len(ch.Name) > maxName || len(ch.Publisher) > maxName:
		return errf(http.StatusBadRequest, "name and publisher must be at most %d bytes", maxName)
	case len(ch.ShopQuery) > maxShopQuery:
		return errf(http.StatusBadRequest, "shop_query must be at most %d bytes", maxShopQuery)
	case len(ch.ApplicationIDs) > maxAppIDs:
		return errf(http.StatusBadRequest, "at most %d application_ids", maxAppIDs)
	}
	return nil
}

func (s *Service) touch() error {
	s.state.SourceRev++
	return s.saveState()
}

// ---------- channels ----------

func (s *Service) listChannels() ([]channelView, error) {
	src, err := s.load()
	if err != nil {
		return nil, err
	}
	out := []channelView{}
	for i := range src.lineup.Channels {
		out = append(out, s.channelView(src, &src.lineup.Channels[i], false, ""))
	}
	var retired []string
	seen := map[string]bool{}
	for _, e := range src.retired.Retired {
		if src.live(e.TopicID) == nil && e.Topic != nil && !seen[e.TopicID] {
			seen[e.TopicID] = true
			retired = append(retired, e.TopicID)
		}
	}
	sort.Strings(retired)
	for _, topic := range retired {
		snap, reason := src.retiredChannel(topic)
		out = append(out, s.channelView(src, snap, true, reason))
	}
	return out, nil
}

func (s *Service) getChannel(topic string) (channelView, error) {
	src, err := s.load()
	if err != nil {
		return channelView{}, err
	}
	if ch := src.live(topic); ch != nil {
		return s.channelView(src, ch, false, ""), nil
	}
	if snap, reason := src.retiredChannel(topic); snap != nil {
		return s.channelView(src, snap, true, reason), nil
	}
	return channelView{}, errf(http.StatusNotFound, "no channel %s", topic)
}

func (s *Service) createChannel(patch map[string]json.RawMessage) (channelView, error) {
	src, err := s.load()
	if err != nil {
		return channelView{}, err
	}
	var topic string
	if raw, ok := patch["topic_id"]; !ok || json.Unmarshal(raw, &topic) != nil || !reAdminTopic.MatchString(topic) {
		return channelView{}, errf(http.StatusBadRequest, "topic_id must match %s", reAdminTopic)
	}
	if snap, _ := src.retiredChannel(topic); src.live(topic) != nil || snap != nil || src.articles[topic] != nil {
		return channelView{}, errf(http.StatusConflict, "topic_id %s already exists; topic ids are never reused", topic)
	}
	if raw, ok := patch["retired"]; ok && string(raw) == "true" {
		return channelView{}, errf(http.StatusBadRequest, "a channel cannot be created retired")
	}
	var ch newsseed.Channel
	base := newsseed.Channel{Icon: newsseed.Art{Motif: "mark"}}
	if err := mergeInto(base, patch, channelReadOnly, &ch); err != nil {
		return channelView{}, err
	}
	if err := checkChannel(&ch); err != nil {
		return channelView{}, err
	}
	if err := os.MkdirAll(filepath.Join(s.src, topic), 0755); err != nil {
		return channelView{}, err
	}
	src.lineup.Channels = append(src.lineup.Channels, ch)
	if err := writeJSON(filepath.Join(s.src, "lineup.json"), src.lineup); err != nil {
		return channelView{}, err
	}
	src.articles[topic] = map[int64]*newsseed.Article{}
	return s.channelView(src, &ch, false, ""), s.touch()
}

func (s *Service) updateChannel(topic string, patch map[string]json.RawMessage) (channelView, error) {
	src, err := s.load()
	if err != nil {
		return channelView{}, err
	}
	if raw, ok := patch["topic_id"]; ok {
		var t string
		if json.Unmarshal(raw, &t) != nil || t != topic {
			return channelView{}, errf(http.StatusBadRequest, "topic_id cannot be changed")
		}
	}
	set, retire, reason, err := retireFlag(patch)
	if err != nil {
		return channelView{}, err
	}
	live := src.live(topic)
	if live == nil {
		snap, _ := src.retiredChannel(topic)
		if snap == nil {
			return channelView{}, errf(http.StatusNotFound, "no channel %s", topic)
		}
		if !set || retire || !onlyKeys(patch, channelReadOnly, "topic_id") {
			return channelView{}, errf(http.StatusConflict, "channel %s is retired; restore it with {\"retired\": false} before editing", topic)
		}
		// Restore: back to the end of the lineup; its articles stay retired.
		src.lineup.Channels = append(src.lineup.Channels, *snap)
		for i := range src.retired.Retired {
			if src.retired.Retired[i].TopicID == topic {
				src.retired.Retired[i].Topic = nil
			}
		}
		if err := s.saveSource(src); err != nil {
			return channelView{}, err
		}
		return s.channelView(src, snap, false, ""), s.touch()
	}
	var updated newsseed.Channel // decoded fresh, so null resets a key
	if err := mergeInto(live, patch, channelReadOnly, &updated); err != nil {
		return channelView{}, err
	}
	updated.TopicID = topic
	if err := checkChannel(&updated); err != nil {
		return channelView{}, err
	}
	if set && retire {
		if len(src.articles[topic]) == 0 {
			return channelView{}, errf(http.StatusConflict, "channel %s has no articles; a channel is retired through its articles' expired replacements, so it needs at least one", topic)
		}
		snap := updated
		for id := range src.articles[topic] {
			if e := src.retiredEntry(topic, id); e != nil {
				e.Topic = &snap
			} else {
				src.retired.Retired = append(src.retired.Retired, newsseed.RetiredEntry{TopicID: topic, NewsID: id, Reason: reason, Topic: &snap})
			}
		}
		var kept []newsseed.Channel
		for _, ch := range src.lineup.Channels {
			if ch.TopicID != topic {
				kept = append(kept, ch)
			}
		}
		src.lineup.Channels = append([]newsseed.Channel{}, kept...)
		if err := s.saveSource(src); err != nil {
			return channelView{}, err
		}
		got, _ := src.retiredChannel(topic)
		_, r := src.retiredChannel(topic)
		return s.channelView(src, got, true, r), s.touch()
	}
	*live = updated
	if err := writeJSON(filepath.Join(s.src, "lineup.json"), src.lineup); err != nil {
		return channelView{}, err
	}
	return s.channelView(src, live, false, ""), s.touch()
}

// saveSource writes retired.json then lineup.json.
func (s *Service) saveSource(src *source) error {
	if src.retired.Retired == nil {
		src.retired.Retired = []newsseed.RetiredEntry{}
	}
	if err := src.retired.Save(filepath.Join(s.src, "retired.json")); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.src, "lineup.json"), src.lineup)
}

// ---------- articles ----------

func (s *Service) channelArticles(src *source, topic string) (map[int64]*newsseed.Article, bool, error) {
	if src.live(topic) != nil {
		return src.articles[topic], true, nil
	}
	if snap, _ := src.retiredChannel(topic); snap != nil {
		return src.articles[topic], false, nil
	}
	return nil, false, errf(http.StatusNotFound, "no channel %s", topic)
}

func (s *Service) listArticles(topic string) ([]articleView, error) {
	src, err := s.load()
	if err != nil {
		return nil, err
	}
	arts, _, err := s.channelArticles(src, topic)
	if err != nil {
		return nil, err
	}
	published := s.publishedIDs()
	out := []articleView{}
	for _, a := range arts {
		out = append(out, s.articleView(src, topic, a, published))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NewsID > out[j].NewsID })
	return out, nil
}

func (s *Service) findArticle(src *source, topic, id string) (*newsseed.Article, bool, error) {
	arts, live, err := s.channelArticles(src, topic)
	if err != nil {
		return nil, false, err
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || arts[n] == nil {
		return nil, false, errf(http.StatusNotFound, "no article %s/%s", topic, id)
	}
	return arts[n], live, nil
}

func (s *Service) getArticle(topic, id string) (articleView, error) {
	src, err := s.load()
	if err != nil {
		return articleView{}, err
	}
	a, _, err := s.findArticle(src, topic, id)
	if err != nil {
		return articleView{}, err
	}
	return s.articleView(src, topic, a, s.publishedIDs()), nil
}

func (s *Service) articlePath(topic string, id int64) string {
	return filepath.Join(s.src, topic, itoa(id)+".json")
}

func checkArticle(a *newsseed.Article) error {
	if err := a.Validate(); err != nil {
		return errf(http.StatusBadRequest, "%v", err)
	}
	return nil
}

func (s *Service) createArticle(topic string, patch map[string]json.RawMessage) (articleView, error) {
	src, err := s.load()
	if err != nil {
		return articleView{}, err
	}
	_, live, err := s.channelArticles(src, topic)
	if err != nil {
		return articleView{}, err
	}
	if !live {
		return articleView{}, errf(http.StatusConflict, "channel %s is retired", topic)
	}
	if raw, ok := patch["retired"]; ok && string(raw) == "true" {
		return articleView{}, errf(http.StatusBadRequest, "an article cannot be created retired")
	}
	var a newsseed.Article
	if err := mergeInto(newsseed.Article{}, patch, articleReadOnly, &a); err != nil {
		return articleView{}, err
	}
	published := s.publishedIDs()
	if a.NewsID == 0 {
		a.NewsID = s.allocateID(src, published)
		if a.NewsID == 0 {
			return articleView{}, errf(http.StatusConflict, "no free news id left today")
		}
	} else if src.usedID(a.NewsID) || publishedAnywhere(published, a.NewsID) {
		return articleView{}, errf(http.StatusConflict, "news_id %d is already used; ids are never reused", a.NewsID)
	}
	if err := checkArticle(&a); err != nil {
		return articleView{}, err
	}
	if err := writeJSON(s.articlePath(topic, a.NewsID), &a); err != nil {
		return articleView{}, err
	}
	src.articles[topic][a.NewsID] = &a
	return s.articleView(src, topic, &a, published), s.touch()
}

func publishedAnywhere(published map[string]bool, id int64) bool {
	suffix := "/" + itoa(id)
	for k := range published {
		if strings.HasSuffix(k, suffix) {
			return true
		}
	}
	return false
}

// allocateID returns YYMMDD (UTC today) + the first free 4-digit sequence.
func (s *Service) allocateID(src *source, published map[string]bool) int64 {
	day, _ := strconv.ParseInt(s.cfg.Now().UTC().Format("060102"), 10, 64)
	for seq := int64(1); seq <= 9999; seq++ {
		id := day*10000 + seq
		if id >= 1<<32 {
			return 0
		}
		if !src.usedID(id) && !publishedAnywhere(published, id) {
			return id
		}
	}
	return 0
}

func (s *Service) updateArticle(topic, id string, patch map[string]json.RawMessage) (articleView, error) {
	src, err := s.load()
	if err != nil {
		return articleView{}, err
	}
	a, live, err := s.findArticle(src, topic, id)
	if err != nil {
		return articleView{}, err
	}
	if raw, ok := patch["news_id"]; ok {
		var n int64
		if json.Unmarshal(raw, &n) != nil || n != a.NewsID {
			return articleView{}, errf(http.StatusBadRequest, "news_id cannot be changed")
		}
	}
	set, retire, reason, err := retireFlag(patch)
	if err != nil {
		return articleView{}, err
	}
	if !live {
		return articleView{}, errf(http.StatusConflict, "channel %s is retired; restore it first", topic)
	}
	entry := src.retiredEntry(topic, a.NewsID)
	published := s.publishedIDs()
	if entry != nil {
		if !set || retire || !onlyKeys(patch, articleReadOnly, "news_id") {
			return articleView{}, errf(http.StatusConflict, "article %s/%d is retired; restore it with {\"retired\": false} before editing", topic, a.NewsID)
		}
		var kept []newsseed.RetiredEntry
		for _, e := range src.retired.Retired {
			if e.TopicID != topic || e.NewsID != a.NewsID {
				kept = append(kept, e)
			}
		}
		src.retired.Retired = kept
		if err := s.saveSource(src); err != nil {
			return articleView{}, err
		}
		return s.articleView(src, topic, a, published), s.touch()
	}
	var updated newsseed.Article
	if err := mergeInto(a, patch, articleReadOnly, &updated); err != nil {
		return articleView{}, err
	}
	updated.NewsID = a.NewsID
	if err := checkArticle(&updated); err != nil {
		return articleView{}, err
	}
	if err := writeJSON(s.articlePath(topic, a.NewsID), &updated); err != nil {
		return articleView{}, err
	}
	src.articles[topic][a.NewsID] = &updated
	if set && retire {
		src.retired.Retired = append(src.retired.Retired, newsseed.RetiredEntry{TopicID: topic, NewsID: a.NewsID, Reason: reason})
		if err := s.saveSource(src); err != nil {
			return articleView{}, err
		}
	}
	return s.articleView(src, topic, &updated, published), s.touch()
}

// deleteArticle removes a never-published article, and retires one that
// shipped (consoles may hold it; its id must stay reserved).
func (s *Service) deleteArticle(topic, id string) (map[string]any, error) {
	src, err := s.load()
	if err != nil {
		return nil, err
	}
	a, live, err := s.findArticle(src, topic, id)
	if err != nil {
		return nil, err
	}
	if !live {
		return nil, errf(http.StatusConflict, "channel %s is retired; restore it first", topic)
	}
	published := s.publishedIDs()
	if published[topic+"/"+itoa(a.NewsID)] {
		if src.retiredEntry(topic, a.NewsID) == nil {
			src.retired.Retired = append(src.retired.Retired, newsseed.RetiredEntry{TopicID: topic, NewsID: a.NewsID, Reason: deletedReason})
			if err := s.saveSource(src); err != nil {
				return nil, err
			}
			if err := s.touch(); err != nil {
				return nil, err
			}
		}
		return map[string]any{"retired": true, "article": s.articleView(src, topic, a, published)}, nil
	}
	if e := src.retiredEntry(topic, a.NewsID); e != nil {
		var kept []newsseed.RetiredEntry
		for _, e := range src.retired.Retired {
			if e.TopicID != topic || e.NewsID != a.NewsID {
				kept = append(kept, e)
			}
		}
		src.retired.Retired = kept
		if err := s.saveSource(src); err != nil {
			return nil, err
		}
	}
	if err := os.Remove(s.articlePath(topic, a.NewsID)); err != nil {
		return nil, err
	}
	removeImage(newsseed.ScreenshotPath(s.src, a.NewsID))
	return map[string]any{"deleted": true}, s.touch()
}

// ---------- images ----------

func imageExists(base string) bool {
	for _, ext := range newsseed.ImageExts {
		if _, err := os.Stat(base + ext); err == nil {
			return true
		}
	}
	return false
}

func removeImage(base string) {
	for _, ext := range newsseed.ImageExts {
		os.Remove(base + ext)
	}
}

func readImage(base string) ([]byte, string, error) {
	for _, ext := range newsseed.ImageExts {
		data, err := os.ReadFile(base + ext)
		if err == nil {
			ct := "image/png"
			if ext != ".png" {
				ct = "image/jpeg"
			}
			return data, ct, nil
		}
	}
	return nil, "", errf(http.StatusNotFound, "no image uploaded")
}

// checkImage accepts a PNG or JPEG within the size and dimension limits
// and returns the extension to store it under.
func checkImage(data []byte) (string, error) {
	ext := ""
	switch http.DetectContentType(data) {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	default:
		return "", errf(http.StatusUnsupportedMediaType, "image must be PNG or JPEG")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", errf(http.StatusBadRequest, "image does not decode")
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxImageSide || cfg.Height > maxImageSide {
		return "", errf(http.StatusBadRequest, "image must be at most %dx%d", maxImageSide, maxImageSide)
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return "", errf(http.StatusBadRequest, "image does not decode")
	}
	return ext, nil
}

func (s *Service) putImage(base string, data []byte) error {
	ext, err := checkImage(data)
	if err != nil {
		return err
	}
	removeImage(base)
	if err := writeAtomic(base+ext, data); err != nil {
		return err
	}
	return s.touch()
}

// imageTarget resolves an image route to its stored path, refusing
// retired targets for writes.
func (s *Service) imageTarget(topic, id string, write bool) (string, error) {
	src, err := s.load()
	if err != nil {
		return "", err
	}
	if id == "" {
		if src.live(topic) == nil {
			if snap, _ := src.retiredChannel(topic); snap == nil {
				return "", errf(http.StatusNotFound, "no channel %s", topic)
			}
			if write {
				return "", errf(http.StatusConflict, "channel %s is retired; restore it first", topic)
			}
		}
		return newsseed.IconPath(s.src, topic), nil
	}
	a, live, err := s.findArticle(src, topic, id)
	if err != nil {
		return "", err
	}
	if write && (!live || src.retiredEntry(topic, a.NewsID) != nil) {
		return "", errf(http.StatusConflict, "article %s/%d or its channel is retired; restore it first", topic, a.NewsID)
	}
	return newsseed.ScreenshotPath(s.src, a.NewsID), nil
}

var errNoKeys = errors.New("signing keys are not configured (NEWS_SIGNING_KEY, NEWS_AES_KEY)")
