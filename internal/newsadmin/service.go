// SPDX-License-Identifier: AGPL-3.0-only
// Package newsadmin is the server-side authoring and publishing path for
// the native Switch News channels (contract: docs/admin-api.md).
//
// The source of truth is a writable data directory:
//
//	<data>/channels/             the channels/ file format newsseed reads
//	    lineup.json, retired.json, <topic>/<news_id>.json,
//	    screenshots/<news_id>.png|jpg, icons/<topic>.png|jpg
//	<data>/builds/<NNNNNN>/seed/     decoded newsseed output (manifest.json)
//	<data>/builds/<NNNNNN>/signed/   the signed fixture tree consoles get
//	<data>/state.json                served build, revision, history
//	<data>/import-manifest.json      optional pre-server manifest
//
// Publishing rebuilds the seed, signs and verifies it in-process, loads it
// as a route table and swaps it in atomically. Keys are read from their
// files at publish time and never logged or returned.
package newsadmin

import (
	"crypto/rsa"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openpak/news/internal/bcatcontainer"
	"github.com/openpak/news/internal/newsexperiment"
	"github.com/openpak/news/internal/newsseed"
)

// publicKeyPEM is a copy of tools/news-patch/openpak-bcat-public.pem, the
// key the console patch installs. Every container is verified against it,
// independently of the signing key (a test keeps the copies identical).
//
//go:embed openpak-bcat-public.pem
var publicKeyPEM []byte

// OpenPakPublicKey returns the committed console verification key.
func OpenPakPublicKey() *rsa.PublicKey {
	k, err := bcatcontainer.ParsePublicKeyPEM(publicKeyPEM)
	if err != nil {
		panic(err)
	}
	return k
}

// Config wires a Service.
type Config struct {
	DataDir string
	// ImportDir is a channels/ directory copied into DataDir/channels when
	// that is still empty. ImportScreenshots also copies its screenshots/.
	ImportDir         string
	ImportScreenshots bool
	// ImportManifest is a previous newsseed manifest.json (the last
	// file-based build) used for overwrite continuity by the first publish.
	ImportManifest string

	SigningKeyPath string // NEWS_SIGNING_KEY
	AESKeyPath     string // NEWS_AES_KEY
	SecretIndex    byte
	VerifyKey      *rsa.PublicKey // nil = OpenPakPublicKey()

	KeepBuilds int // default 10, minimum 2

	// StaticDir is the fixture tree served until the first publish.
	StaticDir string
	Fallback  http.Handler // the CDN's unknown-route fallback
	Switch    *newsexperiment.Switch

	Push   *Pusher // nil = pushing disabled
	Logger *slog.Logger
	Now    func() time.Time
}

// Service owns the data directory. One mutex serialises every edit,
// publish and rollback.
type Service struct {
	cfg   Config
	log   *slog.Logger
	src   string
	mu    sync.Mutex
	state state
	push  sync.WaitGroup
}

type buildInfo struct {
	Build     int       `json:"build"`
	CreatedAt time.Time `json:"created_at"`
	// Containers, Channels, Articles summarise the build.
	Containers int    `json:"containers"`
	Channels   int    `json:"channels"`
	Articles   int    `json:"articles"`
	Note       string `json:"note"`
}

type publishInfo struct {
	Build         int       `json:"build"`
	At            time.Time `json:"at"`
	ChangedTopics []string  `json:"changed_topics"`
	Containers    int       `json:"containers"`
	Note          string    `json:"note"`
}

type errInfo struct {
	At    time.Time `json:"at"`
	Op    string    `json:"op"`
	Error string    `json:"error"`
}

// PushResult is one nx-baas push attempt.
type PushResult struct {
	TopicID  string    `json:"topic_id"`
	Revision int64     `json:"revision"`
	At       time.Time `json:"at"`
	OK       bool      `json:"ok"`
	Status   int       `json:"status"`
	Error    string    `json:"error"`
}

// state is persisted as state.json.
type state struct {
	NextBuild    int           `json:"next_build"`
	Revision     int64         `json:"revision"`
	Serving      int           `json:"serving"` // 0 = the static tree
	ServingSince time.Time     `json:"serving_since"`
	History      []int         `json:"history"` // served builds, oldest first
	Builds       []buildInfo   `json:"builds"`  // kept builds, oldest first
	SourceRev    int64         `json:"source_rev"`
	BuildRevs    map[int]int64 `json:"build_source_revs"`
	LastPublish  *publishInfo  `json:"last_publish"`
	LastError    *errInfo      `json:"last_error"`
	Pushes       []PushResult  `json:"pushes"`
	// ImportedIDs ("topic/id") came from the file-based lineup, which was
	// already served; they count as published.
	ImportedIDs []string `json:"imported_ids"`
}

// New opens (and on first start, imports into) the data directory and
// stores the tree to serve in cfg.Switch.
func New(cfg Config) (*Service, error) {
	if cfg.DataDir == "" || cfg.Switch == nil {
		return nil, errors.New("newsadmin: DataDir and Switch are required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.KeepBuilds == 0 {
		cfg.KeepBuilds = 10
	}
	if cfg.KeepBuilds < 2 {
		cfg.KeepBuilds = 2
	}
	if cfg.VerifyKey == nil {
		cfg.VerifyKey = OpenPakPublicKey()
	}
	s := &Service{cfg: cfg, log: cfg.Logger, src: filepath.Join(cfg.DataDir, "channels")}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "builds"), 0755); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(s.statePath())
	switch {
	case errors.Is(err, fs.ErrNotExist):
		s.state = state{NextBuild: 1}
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(raw, &s.state); err != nil {
			return nil, fmt.Errorf("state.json: %w", err)
		}
	}
	if s.state.BuildRevs == nil {
		s.state.BuildRevs = map[int]int64{}
	}
	if err := s.importSource(); err != nil {
		return nil, err
	}
	if err := s.saveState(); err != nil {
		return nil, err
	}
	s.cleanBuilds()
	if err := s.serveInitial(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) statePath() string { return filepath.Join(s.cfg.DataDir, "state.json") }

func (s *Service) buildDir(n int) string {
	return filepath.Join(s.cfg.DataDir, "builds", fmt.Sprintf("%06d", n))
}

func (s *Service) saveState() error {
	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.statePath(), append(raw, '\n'))
}

// serveInitial loads the served build, else the static tree.
func (s *Service) serveInitial() error {
	if n := s.state.Serving; n != 0 {
		h, err := newsexperiment.Handler(filepath.Join(s.buildDir(n), "signed"), s.log, s.cfg.Fallback)
		if err == nil {
			s.cfg.Switch.Store(h)
			s.log.Info("news admin: serving build", "build", n)
			return nil
		}
		if s.cfg.StaticDir == "" {
			return fmt.Errorf("served build %d: %w", n, err)
		}
		s.log.Error("news admin: served build does not load; falling back to the static tree", "build", n, "error", err)
	}
	if s.cfg.StaticDir == "" {
		s.log.Warn("news admin: no build published and no static tree; console routes answer 503 until the first publish")
		return nil
	}
	h, err := newsexperiment.Handler(s.cfg.StaticDir, s.log, s.cfg.Fallback)
	if err != nil {
		return fmt.Errorf("static tree: %w", err)
	}
	s.cfg.Switch.Store(h)
	return nil
}

// cleanBuilds removes build directories no state entry records (a publish
// interrupted by a crash).
func (s *Service) cleanBuilds() {
	kept := map[string]bool{}
	for _, b := range s.state.Builds {
		kept[filepath.Base(s.buildDir(b.Build))] = true
	}
	entries, _ := os.ReadDir(filepath.Join(s.cfg.DataDir, "builds"))
	for _, e := range entries {
		if !kept[e.Name()] {
			s.log.Warn("news admin: removing unrecorded build directory", "dir", e.Name())
			os.RemoveAll(filepath.Join(s.cfg.DataDir, "builds", e.Name()))
		}
	}
}

// importSource fills an empty data directory from cfg.ImportDir (or an
// empty lineup) and records the imported ids as already published.
func (s *Service) importSource() error {
	if _, err := os.Stat(filepath.Join(s.src, "lineup.json")); err == nil {
		return nil
	}
	if err := os.MkdirAll(s.src, 0755); err != nil {
		return err
	}
	if s.cfg.ImportManifest != "" {
		raw, err := os.ReadFile(s.cfg.ImportManifest)
		if err != nil {
			return fmt.Errorf("import manifest: %w", err)
		}
		var m newsseed.Manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("import manifest: %w", err)
		}
		if err := writeAtomic(filepath.Join(s.cfg.DataDir, "import-manifest.json"), raw); err != nil {
			return err
		}
	}
	if s.cfg.ImportDir == "" {
		if err := writeJSON(filepath.Join(s.src, "retired.json"), newsseed.Retired{Retired: []newsseed.RetiredEntry{}}); err != nil {
			return err
		}
		s.log.Info("news admin: empty data directory; starting with an empty lineup")
		return writeJSON(filepath.Join(s.src, "lineup.json"), newsseed.Lineup{Channels: []newsseed.Channel{}})
	}
	// Validate the import before copying anything.
	if _, err := newsseed.LoadLineup(s.cfg.ImportDir); err != nil {
		return fmt.Errorf("import: %w", err)
	}
	entries, err := os.ReadDir(s.cfg.ImportDir)
	if err != nil {
		return err
	}
	var ids []string
	for _, e := range entries {
		from := filepath.Join(s.cfg.ImportDir, e.Name())
		switch {
		case !e.IsDir() && (e.Name() == "retired.json"):
			if err := copyFile(from, filepath.Join(s.src, e.Name())); err != nil {
				return err
			}
		case e.IsDir() && (e.Name() == "icons" || (e.Name() == "screenshots" && s.cfg.ImportScreenshots)):
			if err := copyDir(from, filepath.Join(s.src, e.Name()), func(string) bool { return true }); err != nil {
				return err
			}
		case e.IsDir() && e.Name() != "screenshots":
			if err := copyDir(from, filepath.Join(s.src, e.Name()), func(n string) bool { return strings.HasSuffix(n, ".json") }); err != nil {
				return err
			}
			files, _ := os.ReadDir(from)
			for _, f := range files {
				if id, ok := strings.CutSuffix(f.Name(), ".json"); ok {
					ids = append(ids, e.Name()+"/"+id)
				}
			}
		}
	}
	if _, err := os.Stat(filepath.Join(s.src, "retired.json")); err != nil {
		if err := writeJSON(filepath.Join(s.src, "retired.json"), newsseed.Retired{Retired: []newsseed.RetiredEntry{}}); err != nil {
			return err
		}
	}
	// lineup.json last: its presence marks a completed import.
	if err := copyFile(filepath.Join(s.cfg.ImportDir, "lineup.json"), filepath.Join(s.src, "lineup.json")); err != nil {
		return err
	}
	sort.Strings(ids)
	s.state.ImportedIDs = ids
	s.log.Info("news admin: imported channel source", "from", s.cfg.ImportDir, "articles", len(ids), "screenshots", s.cfg.ImportScreenshots)
	return nil
}

// ---------- file helpers ----------

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return err
	}
	if err := os.Chmod(f.Name(), 0644); err != nil {
		os.Remove(f.Name())
		return err
	}
	return os.Rename(f.Name(), path)
}

// writeJSON writes v as indented JSON, without HTML escaping, atomically.
func writeJSON(path string, v any) error {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return writeAtomic(path, []byte(b.String()))
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	return writeAtomic(to, data)
}

func copyDir(from, to string, keep func(string) bool) error {
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(to, 0755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !e.Type().IsRegular() || !keep(e.Name()) {
			continue
		}
		if err := copyFile(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// ParseVerifyKey reads a public key PEM for Config.VerifyKey.
func ParseVerifyKey(pem []byte) (*rsa.PublicKey, error) { return bcatcontainer.ParsePublicKeyPEM(pem) }
