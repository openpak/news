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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openpak/news/internal/newsseed"
)

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

	l, err := newsseed.LoadLineup(*source)
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
				if a, err := newsseed.LoadArticle(filepath.Join(dir, e.Name())); err == nil {
					known[ch.TopicID][a.NewsID] = true
				}
			}
		}
	}

	retiredPath := filepath.Join(*source, "retired.json")
	r, err := newsseed.LoadRetired(retiredPath)
	if err != nil {
		return err
	}
	mutated := false
	for _, u := range unretire {
		if !r.Has(u.topic, u.newsID) {
			return fmt.Errorf("--unretire %s/%d: not retired", u.topic, u.newsID)
		}
		var kept []newsseed.RetiredEntry
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
		if r.Has(t.topic, t.newsID) {
			return fmt.Errorf("--retire %s/%d: already retired", t.topic, t.newsID)
		}
		r.Retired = append(r.Retired, newsseed.RetiredEntry{TopicID: t.topic, NewsID: t.newsID,
			Reason: "retired by newsseed; add the reason to channels/retired.json"})
		mutated = true
		fmt.Printf("retired %s/%d\n", t.topic, t.newsID)
	}
	if mutated {
		if err := r.Save(retiredPath); err != nil {
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
	r, err = newsseed.LoadRetired(retiredPath)
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
	if _, err := newsseed.Build(*source, *out, when, newsseed.Options{Adopt: *adopt}); err != nil {
		return err
	}
	fmt.Printf("Seed built into %s (published_at %s).\n", *out, when.Format(time.RFC3339))
	fmt.Println("Next: sign with tools/news-cdn/sign-seed.sh; the CDN never receives keys.")
	return nil
}

func printInventory(source string, l *newsseed.Lineup, r *newsseed.Retired) error {
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
		byID := map[int64]*newsseed.Article{}
		for _, e := range files {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			a, err := newsseed.LoadArticle(filepath.Join(dir, e.Name()))
			if err != nil {
				return err
			}
			ids = append(ids, a.NewsID)
			byID[a.NewsID] = a
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			state := "active"
			if r.Has(topic, id) {
				state = "RETIRED"
			} else if !inLineup[topic] {
				state = "UNRETIRED (channel removed)"
			}
			fmt.Printf("%s\t%d\t%s\t%s\n", topic, id, state, byID[id].Title)
		}
	}
	return nil
}
