// SPDX-License-Identifier: AGPL-3.0-only
package newsseed

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openpak/news/internal/newsformat"
)

// Optional record windows: pinned published_at, priority, pickup_days and
// essential_pickup reach the record; defaults stay as before.
func TestArticleWindows(t *testing.T) {
	src := writeSource(t)
	os.WriteFile(filepath.Join(src, "openpak_test/2609100002.json"), []byte(`{
		"news_id": 2609100002, "title": "Second", "body": "Body two.",
		"language": "en-GB", "supported_languages": ["en-GB"], "lifetime_days": 7,
		"art": {"motif": "orbit"}, "published_at": "2026-09-01T00:00:00Z",
		"priority": 1500, "pickup_days": 2,
		"essential_pickup": {"pickup_days": 1, "priority_after": 200}}`), 0644)
	out := t.TempDir()
	buildDir(t, src, out)
	pub := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Unix()
	rec := decodeFile(t, filepath.Join(out, "openpak_test/2609100002/article.msgpack")).(newsformat.Map)
	ess := get(rec, "essential_pickup").(newsformat.Map)
	for key, want := range map[string]any{
		"published_at": pub, "expire_at": pub + 7*86400, "pickup_limit": pub + 2*86400, "priority": int64(1500),
	} {
		if got := get(rec, key); got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	if get(ess, "pickup_limit") != pub+86400 || get(ess, "priority_after") != int64(200) {
		t.Errorf("essential_pickup = %v", ess)
	}
	def := decodeFile(t, filepath.Join(out, "openpak_test/2609100001/article.msgpack")).(newsformat.Map)
	if get(def, "priority") != int64(100) || get(def, "pickup_limit") != get(def, "expire_at") ||
		get(get(def, "essential_pickup").(newsformat.Map), "pickup_limit") != int64(0) {
		t.Errorf("defaults changed: %v", def)
	}
	for _, bad := range []string{`"priority": 50`, `"pickup_days": 8`, `"published_at": "yesterday"`} {
		a := articleSource{NewsID: 1, Title: "t", Body: "b", Language: "en-GB", SupportedLanguages: []string{"en-GB"},
			LifetimeDays: 7, Art: artSpec{Motif: "orbit"}}
		switch bad {
		case `"priority": 50`:
			a.Priority = 50
		case `"pickup_days": 8`:
			n := 8
			a.PickupDays = &n
		default:
			a.PublishedAt = "yesterday"
		}
		if a.validate() == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}
