// SPDX-License-Identifier: AGPL-3.0-only
// newsseed builds the OpenPak channel lineup for the native Switch News
// experiment from editable source files in channels/.
//
// The build is deterministic: with --published pinned, rerunning it
// reproduces byte-identical payloads, and article identities (news_id) are
// stable, so a rerun updates existing seed content instead of duplicating
// it. Retirement is scoped per article via channels/retired.json. The tool
// produces decoded MessagePack and JPEG only; wrapping into signed BCAT
// containers is done separately (tools/news-cdn/sign-seed.sh) with keys
// that never enter the repository.
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openpak/news/internal/bcatcontainer"
	"github.com/openpak/news/internal/newsformat"
)

var (
	reTopicID   = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)
	reLocale    = regexp.MustCompile(`^[a-z]{2}-[A-Z]{2}$`)
	reNewsIDDir = regexp.MustCompile(`^[0-9]+$`)
	reTitleID   = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

// reservedDirs are output directory names that are not topics: a channel
// may not take them as its topic_id.
var reservedDirs = map[string]bool{"retired": true, "titles": true}

// maxListURL is the list parser's url buffer (256 bytes).
const maxListURL = 256

type artSpec struct {
	Motif string `json:"motif"`
	Hue   int    `json:"hue"`
}

type channelSource struct {
	TopicID     string  `json:"topic_id"`
	Name        string  `json:"name"`
	Publisher   string  `json:"publisher"`
	Description string  `json:"description"`
	Important   bool    `json:"important"`
	Icon        artSpec `json:"icon"`
	// ShopQuery is the channel page's eShop button query (detail
	// shop_query), passed to the shop applet verbatim. Optional.
	ShopQuery string `json:"shop_query,omitempty"`
	// ApplicationIDs are the titles (16 lowercase hex digits) whose launch
	// auto-subscribes the console to this channel. Optional.
	ApplicationIDs []string `json:"application_ids,omitempty"`
	// TestDistribution sets the list's test_distribution flag, under which
	// the module ignores its received-id history. Test channels only.
	TestDistribution bool `json:"test_distribution,omitempty"`
}

type lineupSource struct {
	Channels []channelSource `json:"channels"`
}

type articleSource struct {
	NewsID             int64     `json:"news_id"`
	Title              string    `json:"title"`
	Body               string    `json:"body"`
	Footer             string    `json:"footer"`
	Language           string    `json:"language"`
	SupportedLanguages []string  `json:"supported_languages"`
	LifetimeDays       int       `json:"lifetime_days"`
	Art                artSpec   `json:"art"`
	More               *moreSpec `json:"more,omitempty"`
}

// moreSpec is the article's optional link button: "game" opens a title
// (application_ids), "shop" opens the eShop with a raw query, "browser"
// opens an https page.
type moreSpec struct {
	Type           string   `json:"type"`
	ApplicationIDs []string `json:"application_ids,omitempty"`
	Query          string   `json:"query,omitempty"`
	URL            string   `json:"url,omitempty"`
}

func (m *moreSpec) check() error {
	if m == nil {
		return nil
	}
	ids, err := parseTitleIDs(m.ApplicationIDs)
	if err != nil {
		return err
	}
	switch m.Type {
	case "game":
		if len(ids) == 0 || len(ids) > 5 {
			return fmt.Errorf("game link needs 1..5 application_ids")
		}
	case "shop":
		if strings.TrimSpace(m.Query) == "" && len(ids) == 0 {
			return fmt.Errorf("shop link needs a query or application_ids")
		}
	case "browser":
		if !strings.HasPrefix(m.URL, "https://") || len(m.URL) > maxListURL {
			return fmt.Errorf("browser link needs an https url of at most %d bytes", maxListURL)
		}
	default:
		return fmt.Errorf("more.type must be game, shop or browser (got %q)", m.Type)
	}
	return nil
}

func (m *moreSpec) format() *newsformat.More {
	if m == nil {
		return nil
	}
	ids, _ := parseTitleIDs(m.ApplicationIDs) // checked at load
	return &newsformat.More{Type: m.Type, ApplicationIDs: ids, Query: m.Query, URL: m.URL}
}

func parseTitleIDs(in []string) ([]uint64, error) {
	var out []uint64
	for _, id := range in {
		if !reTitleID.MatchString(id) {
			return nil, fmt.Errorf("application id %q must be 16 lowercase hex digits", id)
		}
		v, _ := strconv.ParseUint(id, 16, 64)
		out = append(out, v)
	}
	return out, nil
}

type retiredEntry struct {
	TopicID string `json:"topic_id"`
	NewsID  int64  `json:"news_id"`
	Reason  string `json:"reason"`
	// Topic is the channel's lineup block, copied here when the channel is
	// removed from lineup.json while this article stays retired. It supplies
	// the payload metadata the lineup no longer provides. Optional while the
	// channel is still in the lineup.
	Topic *channelSource `json:"topic,omitempty"`
}

type retiredFile struct {
	Retired []retiredEntry `json:"retired"`
}

// ---------- id list flag ----------

type idList []struct {
	topic  string
	newsID int64
}

func (l *idList) String() string { return "" }

func (l *idList) Set(v string) error {
	topic, id, ok := strings.Cut(v, "/")
	if !ok {
		return fmt.Errorf("expected topic/news_id, got %q", v)
	}
	nid, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return fmt.Errorf("news_id in %q is not an integer", v)
	}
	*l = append(*l, struct {
		topic  string
		newsID int64
	}{topic, nid})
	return nil
}

// ---------- source loading ----------

func loadLineup(dir string) (*lineupSource, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "lineup.json"))
	if err != nil {
		return nil, err
	}
	var l lineupSource
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, fmt.Errorf("lineup.json: %w", err)
	}
	seen := map[string]bool{}
	for i, ch := range l.Channels {
		switch {
		case !reTopicID.MatchString(ch.TopicID):
			return nil, fmt.Errorf("channel %d: topic_id %q must match %s", i, ch.TopicID, reTopicID)
		case reservedDirs[ch.TopicID]:
			return nil, fmt.Errorf("channel %d: topic_id %q is a reserved output name", i, ch.TopicID)
		case seen[ch.TopicID]:
			return nil, fmt.Errorf("duplicate topic_id %q", ch.TopicID)
		case strings.TrimSpace(ch.Name) == "" || strings.TrimSpace(ch.Publisher) == "":
			return nil, fmt.Errorf("channel %s: name and publisher are required", ch.TopicID)
		case strings.TrimSpace(ch.Description) == "" || len(ch.Description) > 120:
			return nil, fmt.Errorf("channel %s: description must be 1..120 chars", ch.TopicID)
		}
		if err := checkIconArt(ch.Icon); err != nil {
			return nil, fmt.Errorf("channel %s icon: %w", ch.TopicID, err)
		}
		if _, err := parseTitleIDs(ch.ApplicationIDs); err != nil {
			return nil, fmt.Errorf("channel %s: %w", ch.TopicID, err)
		}
		seen[ch.TopicID] = true
	}
	// An empty lineup is legal (every channel removed); the identity scan
	// below still validates whatever article directories remain.
	return &l, nil
}

func loadArticle(path string) (*articleSource, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var a articleSource
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	base := filepath.Base(path)
	if base != strconv.FormatInt(a.NewsID, 10)+".json" {
		return nil, fmt.Errorf("%s: file name must be the article news_id", path)
	}
	switch {
	case a.NewsID <= 0 || a.NewsID >= 1<<32:
		return nil, fmt.Errorf("%s: news_id must be 1..2^32-1", path)
	case strings.TrimSpace(a.Title) == "" || len(a.Title) > 100:
		return nil, fmt.Errorf("%s: title must be 1..100 chars", path)
	case strings.TrimSpace(a.Body) == "" || len(a.Body) > 1200:
		return nil, fmt.Errorf("%s: body must be 1..1200 chars", path)
	case strings.ContainsAny(a.Body, "\n\r"):
		return nil, fmt.Errorf("%s: body must be a single paragraph (newlines are unverified in the native reader)", path)
	case len(a.Footer) > 80:
		return nil, fmt.Errorf("%s: footer must be <= 80 chars", path)
	case !reLocale.MatchString(a.Language):
		return nil, fmt.Errorf("%s: language must look like en-GB", path)
	case a.LifetimeDays < 1 || a.LifetimeDays > 30:
		return nil, fmt.Errorf("%s: lifetime_days must be 1..30", path)
	}
	hasSelf := false
	for _, l := range a.SupportedLanguages {
		if !reLocale.MatchString(l) {
			return nil, fmt.Errorf("%s: supported_languages entry %q must look like en-GB", path, l)
		}
		if l == a.Language {
			hasSelf = true
		}
	}
	if !hasSelf {
		return nil, fmt.Errorf("%s: supported_languages must include the article language", path)
	}
	if err := checkSceneArt(a.Art); err != nil {
		return nil, fmt.Errorf("%s: art: %w", path, err)
	}
	if err := a.More.check(); err != nil {
		return nil, fmt.Errorf("%s: more: %w", path, err)
	}
	return &a, nil
}

// checkIconArt validates a channel icon motif (the official mark or a
// dot-variant glyph).
func checkIconArt(a artSpec) error {
	switch a.Motif {
	case "mark", "mark-pad", "mark-question", "mark-gear":
		return nil
	}
	return fmt.Errorf("icon motif must be one of mark, mark-pad, mark-question, mark-gear (got %q)", a.Motif)
}

// checkSceneArt validates an article art motif.
func checkSceneArt(a artSpec) error {
	if _, _, err := sceneByName(a.Motif); err != nil {
		return err
	}
	return nil
}

func loadRetired(path string) (*retiredFile, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &retiredFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var r retiredFile
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &r, nil
}

func (r *retiredFile) save(path string) error {
	sort.Slice(r.Retired, func(i, j int) bool {
		if r.Retired[i].TopicID != r.Retired[j].TopicID {
			return r.Retired[i].TopicID < r.Retired[j].TopicID
		}
		return r.Retired[i].NewsID < r.Retired[j].NewsID
	})
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0644)
}

func (r *retiredFile) has(topic string, id int64) bool {
	for _, e := range r.Retired {
		if e.TopicID == topic && e.NewsID == id {
			return true
		}
	}
	return false
}

// ---------- manifest ----------

type fileInfo struct {
	Path   string `json:"path"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

type articleEntry struct {
	NewsID      int64  `json:"news_id"`
	Title       string `json:"title"`
	State       string `json:"state"`
	PublishedAt int64  `json:"published_at"`
	ExpireAt    int64  `json:"expire_at"`
	Art         string `json:"art"`
	Image       string `json:"image,omitempty"` // "screenshot" or "generated"
	Reason      string `json:"reason,omitempty"`
	// ContentSHA256 hashes the live payload with timestamps zeroed, so a
	// rebuild at a new --published does not count as a content change.
	ContentSHA256 string `json:"content_sha256,omitempty"`
	// Overwrite is the list entry's overwrite flag. The console skips a
	// news_id it has already received unless overwrite is set, so once an
	// article's content changes (or it is retired or unretired) the flag
	// stays set: a console that missed one list revision still takes it.
	Overwrite bool                `json:"overwrite,omitempty"`
	Files     map[string]fileInfo `json:"files"`
}

type channelEntry struct {
	TopicID        string              `json:"topic_id"`
	Name           string              `json:"name"`
	Description    string              `json:"description"`
	PublishingTime int64               `json:"publishing_time"`
	LastPostedAt   int64               `json:"last_posted_at"`
	Files          map[string]fileInfo `json:"files"`
	Articles       []articleEntry      `json:"articles"`
}

type manifest struct {
	Format      int              `json:"format"`
	GeneratedAt string           `json:"generated_at_rfc3339"`
	PublishedAt int64            `json:"published_at"`
	Catalog     fileInfo         `json:"catalog"`
	Channels    []channelEntry   `json:"channels"`
	Removed     []removedChannel `json:"removed_channels"`
	// Titles maps a title id to the topics its launch subscribes to
	// (titles/<tid>.msgpack); titles/none.msgpack answers every other id.
	Titles map[string][]string `json:"titles,omitempty"`
}

// removedChannel records retirement fixtures for a channel that is no
// longer in lineup.json. Their payloads come from the topic snapshot in
// each retired.json entry, so identities stay serviceable after removal.
type removedChannel struct {
	TopicID  string         `json:"topic_id"`
	List     *fileInfo      `json:"list,omitempty"`
	Articles []articleEntry `json:"articles"`
}

// markerName marks an output directory as created by this tool. Pruning
// only ever happens in directories carrying this marker, and only for
// paths recorded in the previous manifest — a directory that newsseed did
// not build is never touched.
const markerName = ".newsseed-managed"

var markerContent = []byte("{\"tool\":\"newsseed\",\"format\":1}\n")

// adoptSweep converts a pre-marker build directory: it removes exactly the
// artifact names this tool generates (nothing else), so a retired article's
// stale live payload cannot survive into the signed tree. Without it, an
// upgraded directory would keep untracked outputs forever, because there is
// no manifest to prune from.
func adoptSweep(out string) error {
	remove := func(rel string) (bool, error) {
		p := filepath.Join(out, rel)
		if _, err := os.Lstat(p); err != nil {
			return false, nil // absent: nothing to sweep
		}
		fmt.Println("adopt: removing", rel)
		return true, os.RemoveAll(p)
	}
	sweep := func(rels ...string) error {
		for _, rel := range rels {
			if _, err := remove(rel); err != nil {
				return err
			}
		}
		return nil
	}
	if err := sweep("retired", "titles", "catalog.msgpack", "manifest.json", markerName); err != nil {
		return err
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || !reTopicID.MatchString(e.Name()) {
			continue
		}
		arts, err := os.ReadDir(filepath.Join(out, e.Name()))
		if err != nil {
			return err
		}
		for _, a := range arts {
			switch {
			case a.IsDir() && reNewsIDDir.MatchString(a.Name()):
				if err := sweep(filepath.Join(e.Name(), a.Name())); err != nil {
					return err
				}
			case !a.IsDir() && (a.Name() == "detail.msgpack" || a.Name() == "icon.jpg" || a.Name() == "archives.msgpack" || a.Name() == "list.msgpack"):
				if err := sweep(filepath.Join(e.Name(), a.Name())); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ---------- build ----------

// dirIsEmpty reports whether the directory exists and has no entries.
func dirIsEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

// pruneStale removes previously built artifacts that this build no longer
// produces. Safety rules:
//
//   - only directories carrying the newsseed marker are pruned at all;
//   - only paths recorded in the previous manifest.json (plus the retired
//     tree, which the marker covers) are removed, so anything newsseed did
//     not create — notes, unrelated checkouts — survives a rerun.
func pruneStale(out string, l *lineupSource, active map[string]map[int64]bool) error {
	if _, err := os.Stat(filepath.Join(out, markerName)); os.IsNotExist(err) {
		return nil // not ours (or built before markers existed): touch nothing
	}
	known := map[string]bool{}
	for _, ch := range l.Channels {
		known[ch.TopicID] = true
	}
	// Removal targets, as paths relative to out.
	targets := map[string]bool{"retired": true, "titles": true}
	if prev := prevManifest(out); prev != nil {
		for _, ch := range prev.Channels {
			if !known[ch.TopicID] {
				targets[ch.TopicID] = true
				continue
			}
			for _, ae := range ch.Articles {
				if !active[ch.TopicID][ae.NewsID] {
					targets[filepath.Join(ch.TopicID, strconv.FormatInt(ae.NewsID, 10))] = true
				}
			}
		}
	}
	for rel := range targets {
		if err := os.RemoveAll(filepath.Join(out, rel)); err != nil {
			return err
		}
	}
	return nil
}

// prevManifest returns the previous build's manifest, or nil when it is
// missing or unreadable (pruning then falls back to the retired tree only).
func prevManifest(out string) *manifest {
	raw, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		return nil
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return &m
}

func writeFile(out string, rel string, data []byte) (fileInfo, error) {
	path := filepath.Join(out, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fileInfo{}, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fileInfo{}, err
	}
	sum := sha256.Sum256(data)
	return fileInfo{Path: rel, Size: len(data), SHA256: hex.EncodeToString(sum[:])}, nil
}

func build(sourceDir, out string, published time.Time, l *lineupSource, r *retiredFile, adopt bool) error {
	// Load and index every article first so identity errors are found
	// before anything is written. The index spans ALL channel directories,
	// including channels no longer in lineup.json: retired identities stay
	// serviceable after their channel is removed.
	type slot struct {
		topic   string
		source  *articleSource
		retired *retiredEntry
	}
	ids := map[int64]string{}                      // news_id -> topic, global uniqueness
	index := map[string]map[int64]*articleSource{} // topic -> id -> source
	retiredByTopic := map[string]map[int64]*retiredEntry{}
	for i := range r.Retired {
		e := &r.Retired[i]
		if retiredByTopic[e.TopicID] == nil {
			retiredByTopic[e.TopicID] = map[int64]*retiredEntry{}
		}
		retiredByTopic[e.TopicID][e.NewsID] = e
	}
	inLineup := map[string]bool{}
	for _, ch := range l.Channels {
		inLineup[ch.TopicID] = true
	}
	loadTopic := func(topic string, requireLineup bool) error {
		dir := filepath.Join(sourceDir, topic)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if requireLineup {
				return fmt.Errorf("channel %s: %w", topic, err)
			}
			return nil // a removed channel may leave no directory at all
		}
		index[topic] = map[int64]*articleSource{}
		found := 0
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			a, err := loadArticle(filepath.Join(dir, e.Name()))
			if err != nil {
				return err
			}
			if owner := ids[a.NewsID]; owner != "" {
				return fmt.Errorf("news_id %d used by both %s and %s", a.NewsID, owner, topic)
			}
			ids[a.NewsID] = topic
			index[topic][a.NewsID] = a
			if _, isRetired := retiredByTopic[topic][a.NewsID]; !isRetired && !requireLineup {
				return fmt.Errorf("article %s/%d is not retired but channel %s is no longer in lineup.json; retire it (with a topic snapshot) or restore the channel",
					topic, a.NewsID, topic)
			}
			found++
		}
		if requireLineup && found == 0 {
			return fmt.Errorf("channel %s has no articles under %s", topic, dir)
		}
		return nil
	}
	for _, ch := range l.Channels {
		if err := loadTopic(ch.TopicID, true); err != nil {
			return err
		}
	}
	extra, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, e := range extra {
		if !e.IsDir() || inLineup[e.Name()] {
			continue
		}
		if !reTopicID.MatchString(e.Name()) {
			return fmt.Errorf("directory %q under %s is not a valid topic id", e.Name(), sourceDir)
		}
		if err := loadTopic(e.Name(), false); err != nil {
			return err
		}
	}

	activeByTopic := map[string]map[int64]bool{}
	var slots []slot
	for _, ch := range l.Channels {
		activeByTopic[ch.TopicID] = map[int64]bool{}
		for id, a := range index[ch.TopicID] {
			s := slot{topic: ch.TopicID, source: a}
			if re, isRetired := retiredByTopic[ch.TopicID][id]; isRetired {
				s.retired = re
			} else {
				activeByTopic[ch.TopicID][id] = true
			}
			slots = append(slots, s)
		}
	}
	for i := range r.Retired {
		e := &r.Retired[i]
		if _, sourced := index[e.TopicID][e.NewsID]; !sourced {
			return fmt.Errorf("retired.json names article %s/%d whose source file is missing; restore channels/%s/%d.json",
				e.TopicID, e.NewsID, e.TopicID, e.NewsID)
		}
		if strings.TrimSpace(e.Reason) == "" {
			return fmt.Errorf("retired entry %s/%d needs a reason", e.TopicID, e.NewsID)
		}
		if !inLineup[e.TopicID] && e.Topic == nil {
			return fmt.Errorf("retired entry %s/%d needs a \"topic\" snapshot now that channel %s is out of lineup.json (copy the channel block there)",
				e.TopicID, e.NewsID, e.TopicID)
		}
	}

	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	// A rerun updates in place, so stale artifacts of articles that were
	// retired, renumbered or removed since the last build must not survive
	// as active outputs (or be signed by sign-seed.sh). Pruning is gated on
	// the marker and the previous manifest (see pruneStale). Directories
	// from before markers existed have no manifest to prune from, so they
	// need an explicit --adopt migration first.
	markerPath := filepath.Join(out, markerName)
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		empty, err := dirIsEmpty(out)
		if err != nil {
			return err
		}
		if !empty {
			if !adopt {
				return fmt.Errorf("%s exists without a newsseed marker (a pre-marker build or a foreign directory); use a fresh --out, or pass --adopt to migrate it (this removes its previously generated seed artifacts first)", out)
			}
			fmt.Printf("adopting %s: sweeping generated seed artifacts\n", out)
			if err := adoptSweep(out); err != nil {
				return err
			}
		}
	} else {
		if adopt {
			fmt.Println("marker already present; --adopt is unnecessary")
		}
		if err := pruneStale(out, l, activeByTopic); err != nil {
			return err
		}
	}
	if err := os.WriteFile(markerPath, markerContent, 0644); err != nil {
		return err
	}

	// The previous build's record of each article drives the list's
	// overwrite flag (read before this build replaces manifest.json).
	prevArticles := map[string]map[int64]articleEntry{}
	if prev := prevManifest(out); prev != nil {
		for _, ch := range prev.Channels {
			prevArticles[ch.TopicID] = map[int64]articleEntry{}
			for _, ae := range ch.Articles {
				prevArticles[ch.TopicID][ae.NewsID] = ae
			}
		}
	}

	man := manifest{
		Format:      1,
		GeneratedAt: published.UTC().Format(time.RFC3339),
		PublishedAt: published.UTC().Unix(),
	}

	// Catalog and per-channel fixtures.
	var catalogChannels []newsformat.Channel
	for _, ch := range l.Channels {
		icon, err := renderJPEG("icon", ch.Icon.Motif, ch.Icon.Hue, 70, 70)
		if err != nil {
			return fmt.Errorf("channel %s icon: %w", ch.TopicID, err)
		}
		var active []slot
		for _, s := range slots {
			if s.topic == ch.TopicID && s.retired == nil {
				active = append(active, s)
			}
		}
		sort.Slice(active, func(i, j int) bool { return active[i].source.NewsID > active[j].source.NewsID })
		var archiveEntries []newsformat.Map
		for _, s := range active {
			archiveEntries = append(archiveEntries, newsformat.ArchiveEntry(s.source.NewsID,
				s.source.Language, articleURL(ch.TopicID, s.source.NewsID, "summary")))
		}
		pubAt, lastAt := published.UTC().Unix(), published.UTC().Unix()
		if len(active) > 0 {
			pubAt, lastAt = active[0].source.stamp(published), active[0].source.stamp(published)
			for _, s := range active {
				t := s.source.stamp(published)
				if t < pubAt {
					pubAt = t
				}
				if t > lastAt {
					lastAt = t
				}
			}
		}
		nfChannel := newsformat.Channel{
			TopicID: ch.TopicID, Name: ch.Name, Publisher: ch.Publisher,
			Description: ch.Description, Important: ch.Important,
			PublishingTime: pubAt, LastPostedAt: lastAt, ShopQuery: ch.ShopQuery,
		}
		catalogChannels = append(catalogChannels, nfChannel)

		entry := channelEntry{TopicID: ch.TopicID, Name: ch.Name,
			Description: ch.Description, PublishingTime: pubAt, LastPostedAt: lastAt,
			Files: map[string]fileInfo{}}
		if fi, err := writeFile(out, filepath.Join(ch.TopicID, "detail.msgpack"),
			mustEncode(newsformat.Detail(nfChannel))); err != nil {
			return err
		} else {
			entry.Files["detail"] = fi
		}
		if fi, err := writeFile(out, filepath.Join(ch.TopicID, "icon.jpg"), icon); err != nil {
			return err
		} else {
			entry.Files["icon"] = fi
		}
		if fi, err := writeFile(out, filepath.Join(ch.TopicID, "archives.msgpack"),
			mustEncode(newsformat.Archives(archiveEntries...))); err != nil {
			return err
		} else {
			entry.Files["archives"] = fi
		}

		var listEntries []newsformat.ListEntry
		for _, s := range slots {
			if s.topic != ch.TopicID {
				continue
			}
			a := s.source
			// A real gameplay capture beats generated art when one exists.
			var card, hero []byte
			imageSource := "generated"
			if shot := loadScreenshot(sourceDir, a.NewsID); shot != nil {
				if card, err = encodeJPEG(coverCrop(shot, 358, 201)); err != nil {
					return fmt.Errorf("article %d card: %w", a.NewsID, err)
				}
				if hero, err = encodeJPEG(coverCrop(shot, 800, 364)); err != nil {
					return fmt.Errorf("article %d hero: %w", a.NewsID, err)
				}
				imageSource = "screenshot"
			} else {
				if card, err = renderJPEG("card", a.Art.Motif, a.Art.Hue, 358, 201); err != nil {
					return fmt.Errorf("article %d card: %w", a.NewsID, err)
				}
				if hero, err = renderJPEG("hero", a.Art.Motif, a.Art.Hue, 800, 364); err != nil {
					return fmt.Errorf("article %d hero: %w", a.NewsID, err)
				}
			}
			payload := newsformat.Article{
				NewsID: a.NewsID, PublishedAt: a.stamp(published),
				ExpireAt: a.stamp(published) + int64(a.LifetimeDays)*86400,
				Language: a.Language, SupportedLanguages: a.SupportedLanguages,
				TopicID: ch.TopicID, TopicName: ch.Name, TopicPublisher: ch.Publisher,
				TopicDescription: ch.Description, TopicImage: icon,
				Title: a.Title, ListImage: card, Footer: a.footer(),
				BodyText: a.Body, BodyImage: hero, BodyImageHeight: 364,
				More: a.More.format(),
			}
			lifetime := int64(a.LifetimeDays) * 86400
			live := contentHash(payload, lifetime, false)
			ae := articleEntry{
				NewsID: a.NewsID, Title: a.Title,
				PublishedAt: payload.PublishedAt, ExpireAt: payload.ExpireAt,
				Art: a.Art.Motif, Image: imageSource, Files: map[string]fileInfo{},
				ContentSHA256: hex.EncodeToString(live[:]),
			}
			dir := filepath.Join(ch.TopicID, strconv.FormatInt(a.NewsID, 10))
			if s.retired != nil {
				ae.State = "retired"
				ae.Reason = s.retired.Reason
				// Retired articles leave the catalog flow; their expired
				// replacement is listed with overwrite so a console that
				// holds the live copy replaces it (then purges it as
				// expired).
				ae.Overwrite = true
				expired := payload.ExpiredPayload()
				body := mustEncode(expired)
				if fi, err := writeFile(out, filepath.Join("retired", dir, "article.msgpack"), body); err != nil {
					return err
				} else {
					ae.Files["article-expired"] = fi
				}
				le, err := listEntry(expired, articleURL(ch.TopicID, a.NewsID, "expired"), body,
					contentHash(payload, lifetime, true), true)
				if err != nil {
					return err
				}
				listEntries = append(listEntries, le)
			} else {
				ae.State = "active"
				prev, seen := prevArticles[ch.TopicID][a.NewsID]
				ae.Overwrite = seen && (prev.Overwrite || prev.State == "retired" ||
					(prev.ContentSHA256 != "" && prev.ContentSHA256 != ae.ContentSHA256))
				body := mustEncode(payload.Payload())
				le, err := listEntry(payload.Payload(), articleURL(ch.TopicID, a.NewsID, "article"), body, live, ae.Overwrite)
				if err != nil {
					return err
				}
				listEntries = append(listEntries, le)
				if fi, err := writeFile(out, filepath.Join(dir, "summary.msgpack"),
					mustEncode(payload.Summary(articleURL(ch.TopicID, a.NewsID, "article")))); err != nil {
					return err
				} else {
					ae.Files["summary"] = fi
				}
				if fi, err := writeFile(out, filepath.Join(dir, "article.msgpack"), body); err != nil {
					return err
				} else {
					ae.Files["article"] = fi
				}
				if fi, err := writeFile(out, filepath.Join(dir, "article-expired.msgpack"),
					mustEncode(payload.ExpiredPayload())); err != nil {
					return err
				} else {
					ae.Files["article-expired"] = fi
				}
				if fi, err := writeFile(out, filepath.Join(dir, "card.jpg"), card); err != nil {
					return err
				} else {
					ae.Files["card"] = fi
				}
				if fi, err := writeFile(out, filepath.Join(dir, "hero.jpg"), hero); err != nil {
					return err
				} else {
					ae.Files["hero"] = fi
				}
			}
			entry.Articles = append(entry.Articles, ae)
		}
		sort.Slice(entry.Articles, func(i, j int) bool {
			return entry.Articles[i].NewsID < entry.Articles[j].NewsID
		})
		// The list is what the News module itself fetches for a
		// subscribed topic; newest first, like the archives.
		sort.Slice(listEntries, func(i, j int) bool { return listEntries[i].NewsID > listEntries[j].NewsID })
		if fi, err := writeFile(out, filepath.Join(ch.TopicID, "list.msgpack"),
			mustEncode(newsformat.List(ch.TopicID, newsformat.InService, ch.TestDistribution, listEntries...))); err != nil {
			return err
		} else {
			entry.Files["list"] = fi
		}
		man.Channels = append(man.Channels, entry)
	}

	// Auto-subscription: titles/<tid>.msgpack per mapped title, and one
	// empty answer for every other title.
	titles := map[string][]string{}
	for _, ch := range l.Channels {
		for _, id := range ch.ApplicationIDs {
			if n := len(titles[id]); n == 0 || titles[id][n-1] != ch.TopicID {
				titles[id] = append(titles[id], ch.TopicID)
			}
		}
	}
	for id, topics := range titles {
		v, err := newsformat.TitleTopics(topics)
		if err != nil {
			return fmt.Errorf("title %s: %w", id, err)
		}
		if _, err := writeFile(out, filepath.Join("titles", id+".msgpack"), mustEncode(v)); err != nil {
			return err
		}
	}
	if _, err := writeFile(out, filepath.Join("titles", "none.msgpack"), mustEncode([]any{})); err != nil {
		return err
	}
	if len(titles) > 0 {
		man.Titles = titles
	}

	// Retirement fixtures for channels no longer in the lineup: their
	// payloads come from the topic snapshot recorded in retired.json.
	for i := range r.Retired {
		e := &r.Retired[i]
		if inLineup[e.TopicID] {
			continue
		}
		a := index[e.TopicID][e.NewsID]
		snap := e.Topic
		if snap.TopicID == "" {
			snap.TopicID = e.TopicID
		}
		if err := checkIconArt(snap.Icon); err != nil {
			return fmt.Errorf("retired entry %s/%d topic snapshot icon: %w", e.TopicID, e.NewsID, err)
		}
		icon, err := renderJPEG("icon", snap.Icon.Motif, snap.Icon.Hue, 70, 70)
		if err != nil {
			return fmt.Errorf("retired entry %s/%d icon: %w", e.TopicID, e.NewsID, err)
		}
		card, err := renderJPEG("card", a.Art.Motif, a.Art.Hue, 358, 201)
		if err != nil {
			return fmt.Errorf("retired entry %s/%d card: %w", e.TopicID, e.NewsID, err)
		}
		hero, err := renderJPEG("hero", a.Art.Motif, a.Art.Hue, 800, 364)
		if err != nil {
			return fmt.Errorf("retired entry %s/%d hero: %w", e.TopicID, e.NewsID, err)
		}
		payload := newsformat.Article{
			NewsID: a.NewsID, PublishedAt: a.stamp(published),
			ExpireAt: a.stamp(published) + int64(a.LifetimeDays)*86400,
			Language: a.Language, SupportedLanguages: a.SupportedLanguages,
			TopicID: e.TopicID, TopicName: snap.Name, TopicPublisher: snap.Publisher,
			TopicDescription: snap.Description, TopicImage: icon,
			Title: a.Title, ListImage: card, Footer: a.footer(),
			BodyText: a.Body, BodyImage: hero, BodyImageHeight: 364,
			More: a.More.format(),
		}
		fi, err := writeFile(out, filepath.Join("retired", e.TopicID,
			strconv.FormatInt(a.NewsID, 10), "article.msgpack"),
			mustEncode(payload.ExpiredPayload()))
		if err != nil {
			return err
		}
		placed := false
		for j := range man.Removed {
			if man.Removed[j].TopicID == e.TopicID {
				man.Removed[j].Articles = append(man.Removed[j].Articles, articleEntry{
					NewsID: a.NewsID, Title: a.Title, State: "retired",
					PublishedAt: payload.PublishedAt, ExpireAt: payload.ExpireAt,
					Art: a.Art.Motif, Reason: e.Reason,
					Files: map[string]fileInfo{"article-expired": fi},
				})
				placed = true
				break
			}
		}
		if !placed {
			man.Removed = append(man.Removed, removedChannel{TopicID: e.TopicID,
				Articles: []articleEntry{{
					NewsID: a.NewsID, Title: a.Title, State: "retired",
					PublishedAt: payload.PublishedAt, ExpireAt: payload.ExpireAt,
					Art: a.Art.Motif, Reason: e.Reason,
					Files: map[string]fileInfo{"article-expired": fi},
				}}})
		}
	}

	// A removed channel keeps answering its list, as expired: that is what
	// retires the topic on a console still subscribed to it.
	for i := range man.Removed {
		topic := man.Removed[i].TopicID
		fi, err := writeFile(out, filepath.Join("retired", topic, "list.msgpack"),
			mustEncode(newsformat.List(topic, newsformat.Expired, false)))
		if err != nil {
			return err
		}
		man.Removed[i].List = &fi
	}

	if fi, err := writeFile(out, "catalog.msgpack", mustEncode(newsformat.Catalog(catalogChannels))); err != nil {
		return err
	} else {
		man.Catalog = fi
	}

	raw, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "manifest.json"), append(raw, '\n'), 0644)
}

func (a *articleSource) stamp(published time.Time) int64 {
	return published.UTC().Unix()
}

// contentHash identifies an article's content independent of the build's
// publication time.
func contentHash(p newsformat.Article, lifetime int64, expired bool) [32]byte {
	p.PublishedAt, p.ExpireAt = 0, lifetime
	if expired {
		return sha256.Sum256(mustEncode(p.ExpiredPayload()))
	}
	return sha256.Sum256(mustEncode(p.Payload()))
}

// listEntry builds the article's bcat-list entry. The module downloads url
// and expects exactly size bytes: the signed container, which is the
// payload plus the fixed container header (internal/bcatcontainer).
// data_id is derived from the content, so it changes when the content does.
func listEntry(record newsformat.Map, url string, body []byte, content [32]byte, overwrite bool) (newsformat.ListEntry, error) {
	var id, published int64
	var lang string
	for _, f := range record {
		switch f.Key {
		case "news_id":
			id = f.Value.(int64)
		case "published_at":
			published = f.Value.(int64)
		case "language":
			lang = f.Value.(string)
		}
	}
	if !strings.HasPrefix(url, "https://") || len(url) > maxListURL {
		return newsformat.ListEntry{}, fmt.Errorf("article %d: list url must be https and at most %d bytes: %s", id, maxListURL, url)
	}
	return newsformat.ListEntry{
		NewsID: id, PublishingTime: published,
		DeletionPriority: newsformat.DeletionPriority, DefaultLanguage: lang,
		Languages: []newsformat.ListLanguage{{
			Language: lang, DataID: int64(binary.BigEndian.Uint64(content[:8]) >> 1),
			URL: url, Size: int64(bcatcontainer.HeaderSize + len(body)), Overwrite: overwrite,
		}},
	}, nil
}

func articleURL(topic string, id int64, resource string) string {
	// Keep News resources on the hostname already redirected by the installer.
	return fmt.Sprintf("https://bcat-topics-lp1.cdn.nintendo.net/openpak/v1/topics/%s/news/%d/%s", topic, id, resource)
}

func (a *articleSource) footer() string {
	if strings.TrimSpace(a.Footer) != "" {
		return a.Footer
	}
	return "Published by OpenPak."
}

func mustEncode(v any) []byte {
	b, err := newsformat.Encode(v)
	if err != nil {
		panic(err) // all seed payloads use supported types only
	}
	return b
}

// renderJPEG renders one artwork at the requested size.
func renderJPEG(kind, motif string, hue, w, h int) ([]byte, error) {
	if kind == "icon" {
		im, err := renderIcon(motif, hue)
		if err != nil {
			return nil, err
		}
		return encodeJPEG(im)
	}
	im, err := renderArt(motif, hue, w, h)
	if err != nil {
		return nil, err
	}
	return encodeJPEG(im)
}

// ---------- command ----------

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "newsseed:", err)
		os.Exit(1)
	}
}

func run() error {
	source := flag.String("source", "channels", "channel source directory")
	out := flag.String("out", "", "build output directory (created or updated in place)")
	published := flag.String("published", "", "pin the publication time, RFC3339 with UTC offset")
	adopt := flag.Bool("adopt", false, "migrate a pre-marker output directory (sweeps its generated seed artifacts)")
	list := flag.Bool("list", false, "print the inventory and exit")
	var retire, unretire idList
	flag.Var(&retire, "retire", "retire article topic/news_id (repeatable)")
	flag.Var(&unretire, "unretire", "return a retired article topic/news_id to the lineup (repeatable)")
	flag.Parse()

	l, err := loadLineup(*source)
	if err != nil {
		return err
	}
	known := map[string]map[int64]bool{}
	for _, ch := range l.Channels {
		known[ch.TopicID] = map[int64]bool{}
		dir := filepath.Join(*source, ch.TopicID)
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				if a, err := loadArticle(filepath.Join(dir, e.Name())); err == nil {
					known[ch.TopicID][a.NewsID] = true
				}
			}
		}
	}

	retiredPath := filepath.Join(*source, "retired.json")
	r, err := loadRetired(retiredPath)
	if err != nil {
		return err
	}
	mutated := false
	for _, u := range unretire {
		if !r.has(u.topic, u.newsID) {
			return fmt.Errorf("--unretire %s/%d: not retired", u.topic, u.newsID)
		}
		var kept []retiredEntry
		for _, e := range r.Retired {
			if e.TopicID != u.topic || e.NewsID != u.newsID {
				kept = append(kept, e)
			}
		}
		r.Retired = kept
		mutated = true
		fmt.Printf("unretired %s/%d\n", u.topic, u.newsID)
	}
	for _, t := range retire {
		if !known[t.topic][t.newsID] {
			return fmt.Errorf("--retire %s/%d: no such article in the lineup", t.topic, t.newsID)
		}
		if r.has(t.topic, t.newsID) {
			return fmt.Errorf("--retire %s/%d: already retired", t.topic, t.newsID)
		}
		r.Retired = append(r.Retired, retiredEntry{TopicID: t.topic, NewsID: t.newsID,
			Reason: "retired by newsseed; add the reason to channels/retired.json"})
		mutated = true
		fmt.Printf("retired %s/%d\n", t.topic, t.newsID)
	}
	if mutated {
		if err := r.save(retiredPath); err != nil {
			return err
		}
		if *out == "" {
			fmt.Println("channels/retired.json updated; run again with --out to rebuild payloads")
		}
	}

	when := time.Now().UTC().Truncate(time.Second)
	if *published != "" {
		when, err = time.Parse(time.RFC3339, *published)
		if err != nil {
			return fmt.Errorf("--published: %w", err)
		}
		if when.Location() != time.UTC {
			return fmt.Errorf("--published must be UTC (Z offset)")
		}
	}

	// Reload retirement state after any mutation so the build always sees
	// what is on disk.
	r, err = loadRetired(retiredPath)
	if err != nil {
		return err
	}

	if *list {
		return printInventory(*source, l, r)
	}
	if *out == "" {
		if !mutated {
			fmt.Fprintln(os.Stderr, "nothing to do: pass --out DIR to build, --list to inspect, or --retire/--unretire")
			os.Exit(2)
		}
		return nil
	}
	if err := build(*source, *out, when, l, r, *adopt); err != nil {
		return err
	}
	fmt.Printf("Seed built into %s (published_at %s).\n", *out, when.Format(time.RFC3339))
	fmt.Println("Next: sign with tools/news-cdn/sign-seed.sh; the CDN never receives keys.")
	return nil
}

func printInventory(source string, l *lineupSource, r *retiredFile) error {
	_ = r
	inLineup := map[string]bool{}
	for _, ch := range l.Channels {
		inLineup[ch.TopicID] = true
	}
	fmt.Println("topic_id\tnews_id\tstate\ttitle")
	var dirs []string
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Strings(dirs)
	for _, topic := range dirs {
		dir := filepath.Join(source, topic)
		files, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		var ids []int64
		byID := map[int64]*articleSource{}
		for _, e := range files {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			a, err := loadArticle(filepath.Join(dir, e.Name()))
			if err != nil {
				return err
			}
			ids = append(ids, a.NewsID)
			byID[a.NewsID] = a
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			state := "active"
			if r.has(topic, id) {
				state = "RETIRED"
			} else if !inLineup[topic] {
				state = "UNRETIRED (channel removed)"
			}
			fmt.Printf("%s\t%d\t%s\t%s\n", topic, id, state, byID[id].Title)
		}
	}
	return nil
}
