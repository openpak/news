package newsadmin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openpak/news/internal/newsexperiment"
)

// Needs a Postgres: NEWS_TEST_DATABASE_URL (a throwaway database). Skipped without it.
func TestStateSurvivesRestart(t *testing.T) {
	url := os.Getenv("NEWS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("NEWS_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := OpenStateDB(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.pool.Exec(ctx, `DELETE FROM news_state`)

	// A legacy state.json is imported once and left in place.
	data := t.TempDir()
	legacy := filepath.Join(data, "state.json")
	os.WriteFile(legacy, []byte(`{"next_build":5,"revision":7,"history":[3,4]}`), 0o600)
	mk := func(data string, db *StateDB) *Service {
		s, err := New(Config{DataDir: data, ImportDir: writeSourceTree(t), Switch: &newsexperiment.Switch{}, StateDB: db})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s1 := mk(data, db)
	if s1.state.Revision != 7 || s1.state.NextBuild != 5 {
		t.Fatalf("import: %+v", s1.state)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("legacy file must stay")
	}
	s1.state.Revision = 8
	if err := s1.saveState(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(legacy, []byte(`{"next_build":1,"revision":1}`), 0o600) // ignored once in the DB

	// Restart: a fresh store over the same database (and an empty data dir).
	db2, err := OpenStateDB(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	s2 := mk(t.TempDir(), db2)
	if s2.state.Revision != 8 || s2.state.NextBuild != 5 || len(s2.state.History) != 2 {
		t.Fatalf("restart: %+v", s2.state)
	}
}
