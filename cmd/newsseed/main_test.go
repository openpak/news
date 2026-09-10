// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
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
			"icon": {"motif": "beacon", "hue": 212}
		}]}`)
	mustWrite("openpak_test/2609100001.json", `{
		"news_id": 2609100001,
		"title": "First",
		"body": "Body one.",
		"language": "en-GB",
		"supported_languages": ["en-GB"],
		"lifetime_days": 7,
		"art": {"motif": "rays", "hue": 160}
	}`)
	mustWrite("openpak_test/2609100002.json", `{
		"news_id": 2609100002,
		"title": "Second",
		"body": "Body two.",
		"language": "en-GB",
		"supported_languages": ["en-GB"],
		"lifetime_days": 7,
		"art": {"motif": "waves", "hue": 200}
	}`)
	mustWrite("retired.json", `{"retired": []}`)
	return dir
}

func buildDir(t *testing.T, source, out string) {
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
	if err := build(source, out, when, l, r); err != nil {
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

func TestSourceValidation(t *testing.T) {
	base := writeSource(t)
	cases := []struct {
		name, rel, body string
	}{
		{"duplicate id", "openpak_test/2609100099.json", `{"news_id": 2609100001, "title": "x",
			"body": "b", "language": "en-GB", "supported_languages": ["en-GB"],
			"lifetime_days": 7, "art": {"motif": "rays", "hue": 1}}`},
		{"filename mismatch", "openpak_test/999.json", `{"news_id": 2609100042, "title": "x",
			"body": "b", "language": "en-GB", "supported_languages": ["en-GB"],
			"lifetime_days": 7, "art": {"motif": "rays", "hue": 1}}`},
		{"newline body", "openpak_test/2609100043.json", `{"news_id": 2609100043, "title": "x",
			"body": "a\nb", "language": "en-GB", "supported_languages": ["en-GB"],
			"lifetime_days": 7, "art": {"motif": "rays", "hue": 1}}`},
		{"unknown motif", "openpak_test/2609100044.json", `{"news_id": 2609100044, "title": "x",
			"body": "b", "language": "en-GB", "supported_languages": ["en-GB"],
			"lifetime_days": 7, "art": {"motif": "flames", "hue": 1}}`},
		{"id too large", "openpak_test/2609100045.json", `{"news_id": 5000000000, "title": "x",
			"body": "b", "language": "en-GB", "supported_languages": ["en-GB"],
			"lifetime_days": 7, "art": {"motif": "rays", "hue": 1}}`},
		{"missing self language", "openpak_test/2609100046.json", `{"news_id": 2609100046,
			"title": "x", "body": "b", "language": "en-GB",
			"supported_languages": ["en-US"], "lifetime_days": 7,
			"art": {"motif": "rays", "hue": 1}}`},
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
			if err := build(dir, filepath.Join(t.TempDir(), "o"), when, l, r); err == nil {
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
	if err := build(source, t.TempDir(), when, l, r); err == nil {
		t.Fatal("retired entry without reason accepted")
	}
}
