// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openpak/news/internal/bcatcontainer"
	"github.com/openpak/news/internal/newsformat"
)

const pinned = "2026-09-10T08:00:00Z"

func writeSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite := func(rel, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("lineup.json", `{
		"channels": [{
			"topic_id": "openpak_test",
			"name": "OpenPak Test",
			"publisher": "OpenPak",
			"description": "Test channel.",
			"important": false,
			"icon": {"motif": "mark"}
		}]}`)
	mustWrite("openpak_test/2609100001.json", `{
		"news_id": 2609100001,
		"title": "First",
		"body": "Body one.",
		"language": "en-GB",
		"supported_languages": ["en-GB"],
		"lifetime_days": 7,
		"art": {"motif": "burst"}
	}`)
	mustWrite("openpak_test/2609100002.json", `{
		"news_id": 2609100002,
		"title": "Second",
		"body": "Body two.",
		"language": "en-GB",
		"supported_languages": ["en-GB"],
		"lifetime_days": 7,
		"art": {"motif": "orbit"}
	}`)
	mustWrite("retired.json", `{"retired": []}`)
	return dir
}

func buildDir(t *testing.T, source, out string) {
	buildDirAdopt(t, source, out, false)
}

func buildDirAdopt(t *testing.T, source, out string, adopt bool) {
	t.Helper()
	l, err := loadLineup(source)
	if err != nil {
		t.Fatal(err)
	}
	r, err := loadRetired(filepath.Join(source, "retired.json"))
	if err != nil {
		t.Fatal(err)
	}
	when, err := time.Parse(time.RFC3339, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if err := build(source, out, when, l, r, adopt); err != nil {
		t.Fatal(err)
	}
}

func TestBuildAndRerunUpdatesInPlace(t *testing.T) {
	source := writeSource(t)
	out := filepath.Join(t.TempDir(), "build")
	buildDir(t, source, out)

	for _, rel := range []string{
		"catalog.msgpack", "manifest.json",
		"openpak_test/detail.msgpack", "openpak_test/icon.jpg", "openpak_test/archives.msgpack",
		"openpak_test/2609100001/article.msgpack", "openpak_test/2609100001/card.jpg",
		"openpak_test/2609100001/hero.jpg", "openpak_test/2609100001/article-expired.msgpack",
		"openpak_test/2609100002/article.msgpack",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Errorf("missing output %s", rel)
		}
	}
	var man manifest
	raw, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &man); err != nil {
		t.Fatal(err)
	}
	if len(man.Channels) != 1 || len(man.Channels[0].Articles) != 2 {
		t.Fatalf("unexpected manifest shape: %+v", man)
	}
	if man.Channels[0].PublishingTime != man.PublishedAt || man.Channels[0].LastPostedAt != man.PublishedAt {
		t.Fatalf("channel window = %d..%d, want %d",
			man.Channels[0].PublishingTime, man.Channels[0].LastPostedAt, man.PublishedAt)
	}

	// A pinned rerun is byte-identical: updates without duplicates.
	var before = map[string][]byte{}
	for _, rel := range []string{"catalog.msgpack", "openpak_test/2609100001/article.msgpack"} {
		b, err := os.ReadFile(filepath.Join(out, rel))
		if err != nil {
			t.Fatal(err)
		}
		before[rel] = b
	}
	buildDir(t, source, out)
	for rel := range before {
		after, err := os.ReadFile(filepath.Join(out, rel))
		if err != nil {
			t.Fatal(err)
		}
		if string(before[rel]) != string(after) {
			t.Errorf("%s changed across a pinned rerun", rel)
		}
	}
}

func TestRetireScope(t *testing.T) {
	source := writeSource(t)
	out := filepath.Join(t.TempDir(), "build")
	// Retirement is a source-file edit (newsseed --retire writes this),
	// then the build reads it from disk.
	if err := os.WriteFile(filepath.Join(source, "retired.json"), []byte(
		`{"retired":[{"topic_id":"openpak_test","news_id":2609100001,"reason":"test retirement"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	buildDir(t, source, out)

	var man manifest
	raw, _ := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err := json.Unmarshal(raw, &man); err != nil {
		t.Fatal(err)
	}
	arts := man.Channels[0].Articles
	if arts[0].State != "retired" || arts[0].Reason != "test retirement" {
		t.Fatalf("article 1 not retired: %+v", arts[0])
	}
	if arts[1].State != "active" {
		t.Fatalf("article 2 not active: %+v", arts[1])
	}
	if _, err := os.Stat(filepath.Join(out, "openpak_test", "2609100001", "article.msgpack")); !os.IsNotExist(err) {
		t.Error("retired article still built as active")
	}
	if _, err := os.Stat(filepath.Join(out, "retired", "openpak_test", "2609100001", "article.msgpack")); err != nil {
		t.Error("retired replacement missing")
	}
	// Channel timestamps follow the remaining active article only; both
	// articles share the pinned time here, so just check the window exists.
	if man.Channels[0].LastPostedAt == 0 {
		t.Error("channel last_posted_at not set")
	}
}

func TestRerunPrunesRetiredArtifacts(t *testing.T) {
	source := writeSource(t)
	out := filepath.Join(t.TempDir(), "build")
	buildDir(t, source, out)
	// Retire one article on disk, then rebuild over the same directory.
	if err := os.WriteFile(filepath.Join(source, "retired.json"), []byte(
		`{"retired":[{"topic_id":"openpak_test","news_id":2609100001,"reason":"test retirement"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	buildDir(t, source, out)
	if _, err := os.Stat(filepath.Join(out, "openpak_test", "2609100001", "article.msgpack")); !os.IsNotExist(err) {
		t.Fatal("stale active artifact survived the retirement rebuild")
	}
	archive, err := os.ReadFile(filepath.Join(out, "openpak_test", "archives.msgpack"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(archive, []byte("/2609100001/summary")) || !bytes.Contains(archive, []byte("/2609100002/summary")) {
		t.Fatal("archive does not exclude retired article while keeping active article")
	}
	if _, err := os.Stat(filepath.Join(out, "openpak_test", "2609100002", "summary.msgpack")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "retired", "openpak_test", "2609100001", "article.msgpack")); err != nil {
		t.Fatal("retired replacement missing")
	}
	// Unretiring (empty file) restores the active artifacts.
	if err := os.WriteFile(filepath.Join(source, "retired.json"), []byte(`{"retired": []}`), 0644); err != nil {
		t.Fatal(err)
	}
	buildDir(t, source, out)
	if _, err := os.Stat(filepath.Join(out, "openpak_test", "2609100001", "article.msgpack")); err != nil {
		t.Fatal("active artifact not restored")
	}
	if _, err := os.Stat(filepath.Join(out, "retired")); !os.IsNotExist(err) {
		t.Fatal("stale retired tree survived")
	}
}

func TestPruneNeverTouchesUnrecordedPaths(t *testing.T) {
	source := writeSource(t)
	out := filepath.Join(t.TempDir(), "build")
	buildDir(t, source, out) // writes the marker and manifest

	// An unrelated directory and file inside the output dir must survive.
	foreign := filepath.Join(out, "notes")
	if err := os.MkdirAll(foreign, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "todo.txt"), []byte("keep me"), 0644); err != nil {
		t.Fatal(err)
	}
	buildDir(t, source, out)
	if _, err := os.Stat(filepath.Join(foreign, "todo.txt")); err != nil {
		t.Fatal("unrelated directory was deleted by a rerun")
	}

	// A directory without the marker (a pre-marker build) must not be
	// silently rebuilt: plain rebuild fails, and --adopt migrates it by
	// sweeping the previously generated artifacts — otherwise a retired
	// article's stale live payload would linger and get signed.
	foreignOut := t.TempDir()
	copyTree(t, out, foreignOut)
	if err := os.Remove(filepath.Join(foreignOut, markerName)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "retired.json"), []byte(
		`{"retired":[{"topic_id":"openpak_test","news_id":2609100001,"reason":"gone"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	l, err := loadLineup(source)
	if err != nil {
		t.Fatal(err)
	}
	r, err := loadRetired(filepath.Join(source, "retired.json"))
	if err != nil {
		t.Fatal(err)
	}
	when, _ := time.Parse(time.RFC3339, pinned)
	if err := build(source, foreignOut, when, l, r, false); err == nil {
		t.Fatal("rebuild of a marker-less directory was accepted")
	}
	// The stale live payload is still there before adoption...
	if _, err := os.Stat(filepath.Join(foreignOut, "openpak_test", "2609100001", "article.msgpack")); err != nil {
		t.Fatal("precondition: stale live payload missing")
	}
	// ...and adoption sweeps it, leaving only the retirement fixture.
	buildDirAdopt(t, source, foreignOut, true)
	if _, err := os.Stat(filepath.Join(foreignOut, "openpak_test", "2609100001", "article.msgpack")); !os.IsNotExist(err) {
		t.Fatal("adopt kept the retired article's stale live payload")
	}
	if _, err := os.Stat(filepath.Join(foreignOut, "retired", "openpak_test", "2609100001", "article.msgpack")); err != nil {
		t.Fatal("adopt lost the retirement fixture")
	}
}

func TestRemovedChannelRetirementStillBuilds(t *testing.T) {
	source := writeSource(t)
	out := filepath.Join(t.TempDir(), "build")
	// Retire both articles, remove the channel from the lineup, and record
	// the topic snapshot in each retirement entry, as documented.
	if err := os.WriteFile(filepath.Join(source, "retired.json"), []byte(`{
		"retired": [
			{"topic_id": "openpak_test", "news_id": 2609100001, "reason": "channel removed",
			 "topic": {"topic_id": "openpak_test", "name": "OpenPak Test", "publisher": "OpenPak",
				"description": "Test channel.", "important": false,
				"icon": {"motif": "mark"}}},
			{"topic_id": "openpak_test", "news_id": 2609100002, "reason": "channel removed",
			 "topic": {"topic_id": "openpak_test", "name": "OpenPak Test", "publisher": "OpenPak",
				"description": "Test channel.", "important": false,
				"icon": {"motif": "mark"}}}
		]
	}`), 0644); err != nil {
		t.Fatal(err)
	}
	var lineup lineupSource
	raw, _ := os.ReadFile(filepath.Join(source, "lineup.json"))
	if err := json.Unmarshal(raw, &lineup); err != nil {
		t.Fatal(err)
	}
	lineup.Channels = nil // drop the channel entirely
	// build() needs the lineup loaded from the source dir, so rewrite it.
	if err := os.WriteFile(filepath.Join(source, "lineup.json"), []byte(`{"channels": []}`), 0644); err != nil {
		t.Fatal(err)
	}
	l, err := loadLineup(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Channels) != 0 {
		t.Fatal("lineup should be empty")
	}
	r, err := loadRetired(filepath.Join(source, "retired.json"))
	if err != nil {
		t.Fatal(err)
	}
	when, _ := time.Parse(time.RFC3339, pinned)
	if err := build(source, out, when, l, r, false); err != nil {
		t.Fatalf("removed-channel retirement rejected: %v", err)
	}
	for _, id := range []string{"2609100001", "2609100002"} {
		if _, err := os.Stat(filepath.Join(out, "retired", "openpak_test", id, "article.msgpack")); err != nil {
			t.Fatalf("retirement fixture for removed channel missing (%s)", id)
		}
	}
	var man manifest
	raw, _ = os.ReadFile(filepath.Join(out, "manifest.json"))
	if err := json.Unmarshal(raw, &man); err != nil {
		t.Fatal(err)
	}
	if len(man.Removed) != 1 || len(man.Removed[0].Articles) != 2 {
		t.Fatalf("manifest removed_channels not recorded: %+v", man.Removed)
	}
	if man.Catalog.Size == 0 {
		t.Fatal("catalog missing")
	}

	// Missing topic snapshot must be rejected with an explanation.
	badOut := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "retired.json"), []byte(
		`{"retired":[{"topic_id":"openpak_test","news_id":2609100001,"reason":"no snapshot"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	r, err = loadRetired(filepath.Join(source, "retired.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := build(source, badOut, when, l, r, false); err == nil {
		t.Fatal("removed-channel retirement without topic snapshot accepted")
	}

	// An unretired article under a removed channel must be rejected too.
	if err := os.WriteFile(filepath.Join(source, "retired.json"), []byte(`{"retired": []}`), 0644); err != nil {
		t.Fatal(err)
	}
	r, err = loadRetired(filepath.Join(source, "retired.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := build(source, badOut, when, l, r, false); err == nil {
		t.Fatal("unretired article under removed channel accepted")
	}
}

func TestSourceValidation(t *testing.T) {
	base := writeSource(t)
	cases := []struct {
		name, rel, body string
	}{
		{"duplicate id", "openpak_test/2609100099.json", `{"news_id": 2609100001, "title": "x",
			"body": "b", "language": "en-GB", "supported_languages": ["en-GB"],
			"lifetime_days": 7, "art": {"motif": "burst"}}`},
		{"filename mismatch", "openpak_test/999.json", `{"news_id": 2609100042, "title": "x",
			"body": "b", "language": "en-GB", "supported_languages": ["en-GB"],
			"lifetime_days": 7, "art": {"motif": "burst"}}`},
		{"newline body", "openpak_test/2609100043.json", `{"news_id": 2609100043, "title": "x",
			"body": "a\nb", "language": "en-GB", "supported_languages": ["en-GB"],
			"lifetime_days": 7, "art": {"motif": "burst"}}`},
		{"unknown motif", "openpak_test/2609100044.json", `{"news_id": 2609100044, "title": "x",
			"body": "b", "language": "en-GB", "supported_languages": ["en-GB"],
			"lifetime_days": 7, "art": {"motif": "flames", "hue": 1}}`},
		{"id too large", "openpak_test/2609100045.json", `{"news_id": 5000000000, "title": "x",
			"body": "b", "language": "en-GB", "supported_languages": ["en-GB"],
			"lifetime_days": 7, "art": {"motif": "burst"}}`},
		{"missing self language", "openpak_test/2609100046.json", `{"news_id": 2609100046,
			"title": "x", "body": "b", "language": "en-GB",
			"supported_languages": ["en-US"], "lifetime_days": 7,
			"art": {"motif": "burst"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			copyTree(t, base, dir)
			if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(tc.rel)), []byte(tc.body), 0644); err != nil {
				t.Fatal(err)
			}
			l, err := loadLineup(dir)
			if err != nil {
				t.Fatal(err)
			}
			r, _ := loadRetired(filepath.Join(dir, "retired.json"))
			when, _ := time.Parse(time.RFC3339, pinned)
			if err := build(dir, filepath.Join(t.TempDir(), "o"), when, l, r, false); err == nil {
				t.Fatalf("%s accepted", tc.name)
			}
		})
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRetiredEntryNeedsReason(t *testing.T) {
	source := writeSource(t)
	rPath := filepath.Join(source, "retired.json")
	if err := os.WriteFile(rPath, []byte(`{"retired":[{"topic_id":"openpak_test","news_id":2609100001,"reason":""}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	l, err := loadLineup(source)
	if err != nil {
		t.Fatal(err)
	}
	r, err := loadRetired(rPath)
	if err != nil {
		t.Fatal(err)
	}
	when, _ := time.Parse(time.RFC3339, pinned)
	if err := build(source, t.TempDir(), when, l, r, false); err == nil {
		t.Fatal("retired entry without reason accepted")
	}
}

// listLanguage returns the single language edition of the list entry for id.
func listLanguage(t *testing.T, list newsformat.Map, id int64) newsformat.Map {
	t.Helper()
	dir := get(list, "directories").([]any)[0].(newsformat.Map)
	for _, e := range get(dir, "data_list").([]any) {
		entry := e.(newsformat.Map)
		if get(entry, "news_id").(int64) == id {
			return get(entry, "languages").([]any)[0].(newsformat.Map)
		}
	}
	t.Fatalf("news_id %d not in list", id)
	return nil
}

func TestListShape(t *testing.T) {
	source := writeSource(t)
	out := filepath.Join(t.TempDir(), "build")
	buildDir(t, source, out)
	list := decodeFile(t, filepath.Join(out, "openpak_test", "list.msgpack")).(newsformat.Map)
	if got := strings.Join(keys(list), ","); got != "topic_id,service_status,na_required,directories" {
		t.Fatalf("list keys = %s", got)
	}
	if get(list, "topic_id") != "openpak_test" || get(list, "service_status") != "in_service" || get(list, "na_required") != false {
		t.Fatal("list header values")
	}
	dirs := get(list, "directories").([]any)
	if len(dirs) != 1 || strings.Join(keys(dirs[0].(newsformat.Map)), ",") != "data_list" {
		t.Fatal("want one directory holding data_list only")
	}
	data := get(dirs[0].(newsformat.Map), "data_list").([]any)
	if len(data) != 2 || get(data[0].(newsformat.Map), "news_id").(int64) != 2609100002 {
		t.Fatal("list must hold both articles, newest first")
	}
	entry := data[1].(newsformat.Map)
	if got := strings.Join(keys(entry), ","); got != "version,news_id,publishing_time,deletion_priority,default_language,languages" {
		t.Fatalf("entry keys = %s", got)
	}
	when, _ := time.Parse(time.RFC3339, pinned)
	if get(entry, "publishing_time").(int64) != when.Unix() || get(entry, "deletion_priority").(int64) != 100 ||
		get(entry, "default_language") != "en-GB" {
		t.Fatal("entry values")
	}
	lang := listLanguage(t, list, 2609100001)
	if got := strings.Join(keys(lang), ","); got != "language,data_id,url,size,overwrite" {
		t.Fatalf("language keys = %s", got)
	}
	article, err := os.ReadFile(filepath.Join(out, "openpak_test", "2609100001", "article.msgpack"))
	if err != nil {
		t.Fatal(err)
	}
	url := get(lang, "url").(string)
	if url != "https://bcat-topics-lp1.cdn.nintendo.net/openpak/v1/topics/openpak_test/news/2609100001/article" || len(url) > 256 {
		t.Fatalf("url = %s", url)
	}
	// size is the signed container: fixed header + payload.
	if get(lang, "size").(int64) != int64(bcatcontainer.HeaderSize+len(article)) {
		t.Fatal("size is not the container size")
	}
	if get(lang, "overwrite") != false || get(lang, "data_id").(int64) <= 0 {
		t.Fatal("fresh article must not overwrite and needs a data_id")
	}
	// The record itself sits in qlaunch's visible priority band.
	record := decodeFile(t, filepath.Join(out, "openpak_test", "2609100001", "article.msgpack")).(newsformat.Map)
	if p := get(record, "priority").(int64); p <= 50 || p >= 1000 {
		t.Fatalf("priority %d outside the main list band", p)
	}
	if get(record, "essential_pickup_limit") != nil || get(record, "essential_pickup") == nil {
		t.Fatal("essential_pickup must be nested")
	}
	detail := decodeFile(t, filepath.Join(out, "openpak_test", "detail.msgpack")).(newsformat.Map)
	if get(detail, "latest_news_urls") != nil || get(detail, "shop_query") != nil {
		t.Fatal("detail carries unused or unset keys")
	}
	// No title mapped: only the empty auto-subscription answer.
	if v := decodeFile(t, filepath.Join(out, "titles", "none.msgpack")).([]any); len(v) != 0 {
		t.Fatal("titles/none must be an empty array")
	}
}

func TestListOverwrite(t *testing.T) {
	source := writeSource(t)
	out := filepath.Join(t.TempDir(), "build")
	buildDir(t, source, out)
	first := decodeFile(t, filepath.Join(out, "openpak_test", "list.msgpack")).(newsformat.Map)
	id1 := get(listLanguage(t, first, 2609100001), "data_id").(int64)

	// A rebuild at a new publication time is not a content change.
	l, _ := loadLineup(source)
	r, _ := loadRetired(filepath.Join(source, "retired.json"))
	later, _ := time.Parse(time.RFC3339, "2026-09-20T08:00:00Z")
	if err := build(source, out, later, l, r, false); err != nil {
		t.Fatal(err)
	}
	list := decodeFile(t, filepath.Join(out, "openpak_test", "list.msgpack")).(newsformat.Map)
	if get(listLanguage(t, list, 2609100001), "overwrite") != false {
		t.Fatal("republishing unchanged content set overwrite")
	}
	if get(listLanguage(t, list, 2609100001), "data_id").(int64) != id1 {
		t.Fatal("data_id changed without a content change")
	}

	// Editing the body replaces the article on consoles that have it.
	path := filepath.Join(source, "openpak_test", "2609100001.json")
	raw, _ := os.ReadFile(path)
	if err := os.WriteFile(path, bytes.Replace(raw, []byte("Body one."), []byte("Body one, corrected."), 1), 0644); err != nil {
		t.Fatal(err)
	}
	buildDir(t, source, out)
	list = decodeFile(t, filepath.Join(out, "openpak_test", "list.msgpack")).(newsformat.Map)
	lang := listLanguage(t, list, 2609100001)
	if get(lang, "overwrite") != true || get(lang, "data_id").(int64) == id1 {
		t.Fatal("changed content must overwrite with a new data_id")
	}
	if get(listLanguage(t, list, 2609100002), "overwrite") != false {
		t.Fatal("untouched article overwrite")
	}
	// Sticky: a later rebuild keeps the flag for consoles that missed it.
	buildDir(t, source, out)
	list = decodeFile(t, filepath.Join(out, "openpak_test", "list.msgpack")).(newsformat.Map)
	if get(listLanguage(t, list, 2609100001), "overwrite") != true {
		t.Fatal("overwrite not sticky")
	}

	// Retiring lists the expired replacement with overwrite.
	if err := os.WriteFile(filepath.Join(source, "retired.json"), []byte(
		`{"retired":[{"topic_id":"openpak_test","news_id":2609100002,"reason":"test"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	buildDir(t, source, out)
	list = decodeFile(t, filepath.Join(out, "openpak_test", "list.msgpack")).(newsformat.Map)
	lang = listLanguage(t, list, 2609100002)
	expired, err := os.ReadFile(filepath.Join(out, "retired", "openpak_test", "2609100002", "article.msgpack"))
	if err != nil {
		t.Fatal(err)
	}
	if get(lang, "overwrite") != true || !strings.HasSuffix(get(lang, "url").(string), "/news/2609100002/expired") ||
		get(lang, "size").(int64) != int64(bcatcontainer.HeaderSize+len(expired)) {
		t.Fatalf("retired entry: %+v", lang)
	}
}

func TestExpiredChannelList(t *testing.T) {
	source := writeSource(t)
	out := filepath.Join(t.TempDir(), "build")
	snap := `"topic": {"topic_id": "openpak_test", "name": "OpenPak Test", "publisher": "OpenPak",
		"description": "Test channel.", "icon": {"motif": "mark"}}`
	if err := os.WriteFile(filepath.Join(source, "retired.json"), []byte(`{"retired": [
		{"topic_id": "openpak_test", "news_id": 2609100001, "reason": "removed", `+snap+`},
		{"topic_id": "openpak_test", "news_id": 2609100002, "reason": "removed", `+snap+`}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "lineup.json"), []byte(`{"channels": []}`), 0644); err != nil {
		t.Fatal(err)
	}
	buildDir(t, source, out)
	list := decodeFile(t, filepath.Join(out, "retired", "openpak_test", "list.msgpack")).(newsformat.Map)
	if get(list, "service_status") != "expired" || get(list, "topic_id") != "openpak_test" {
		t.Fatalf("removed channel list: %+v", list)
	}
	if _, err := os.Stat(filepath.Join(out, "openpak_test")); !os.IsNotExist(err) {
		t.Fatal("removed channel still has live outputs")
	}
}

func TestTitlesAndLinks(t *testing.T) {
	source := writeSource(t)
	if err := os.WriteFile(filepath.Join(source, "lineup.json"), []byte(`{"channels": [{
		"topic_id": "openpak_test", "name": "OpenPak Test", "publisher": "OpenPak",
		"description": "Test channel.", "icon": {"motif": "mark"},
		"shop_query": "dst_nsuid=70010000000001",
		"application_ids": ["0100abcd12340000", "0100abcd12340000"]}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(source, "openpak_test", "2609100001.json")
	raw, _ := os.ReadFile(path)
	withMore := bytes.Replace(raw, []byte(`"lifetime_days"`),
		[]byte(`"more": {"type": "game", "application_ids": ["0100abcd12340000"]}, "lifetime_days"`), 1)
	if err := os.WriteFile(path, withMore, 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "build")
	buildDir(t, source, out)
	topics := decodeFile(t, filepath.Join(out, "titles", "0100abcd12340000.msgpack")).([]any)
	if len(topics) != 1 || topics[0] != "openpak_test" {
		t.Fatalf("title topics = %v", topics)
	}
	detail := decodeFile(t, filepath.Join(out, "openpak_test", "detail.msgpack")).(newsformat.Map)
	if get(detail, "shop_query") != "dst_nsuid=70010000000001" {
		t.Fatal("shop_query missing from detail")
	}
	record := decodeFile(t, filepath.Join(out, "openpak_test", "2609100001", "article.msgpack")).(newsformat.Map)
	more, _ := get(record, "more").(newsformat.Map)
	if strings.Join(keys(more), ",") != "type,application_ids" || get(more, "application_ids").([]any)[0].(int64) != 0x0100abcd12340000 {
		t.Fatalf("more = %+v", more)
	}

	for name, bad := range map[string]string{
		"type":    `"more": {"type": "system_applet"},`,
		"game":    `"more": {"type": "game"},`,
		"browser": `"more": {"type": "browser", "url": "http://x"},`,
		"id":      `"more": {"type": "game", "application_ids": ["0100ABCD12340000"]},`,
	} {
		if err := os.WriteFile(path, bytes.Replace(raw, []byte(`"lifetime_days"`), []byte(bad+`"lifetime_days"`), 1), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := loadArticle(path); err == nil {
			t.Fatalf("bad more (%s) accepted", name)
		}
	}
}
