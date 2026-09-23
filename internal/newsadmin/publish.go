// SPDX-License-Identifier: AGPL-3.0-only
package newsadmin

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/openpak/news/internal/bcatcontainer"
	"github.com/openpak/news/internal/newsexperiment"
	"github.com/openpak/news/internal/newsseed"
)

// PublishResult is the publish / rollback answer.
type PublishResult struct {
	Build         int      `json:"build"`
	Revision      int64    `json:"revision"`
	PublishedAt   string   `json:"published_at"`
	Containers    int      `json:"containers"`
	Channels      int      `json:"channels"`
	Articles      int      `json:"articles"`
	ChangedTopics []string `json:"changed_topics"`
	Push          string   `json:"push"`
	Note          string   `json:"note"`
}

func (s *Service) keysConfigured() bool {
	if s.cfg.SigningKeyPath == "" || s.cfg.AESKeyPath == "" {
		return false
	}
	_, err1 := os.Stat(s.cfg.SigningKeyPath)
	_, err2 := os.Stat(s.cfg.AESKeyPath)
	return err1 == nil && err2 == nil
}

// loadKeys reads both keys and checks that the signing key belongs to
// the verification key. Errors never carry key material.
func (s *Service) loadKeys() (*rsa.PrivateKey, []byte, error) {
	if !s.keysConfigured() {
		return nil, nil, errf(http.StatusServiceUnavailable, "%v", errNoKeys)
	}
	pem, err := os.ReadFile(s.cfg.SigningKeyPath)
	if err != nil {
		return nil, nil, errf(http.StatusServiceUnavailable, "signing key unreadable")
	}
	signing, err := bcatcontainer.ParsePrivateKeyPEM(pem)
	if err != nil {
		return nil, nil, errf(http.StatusServiceUnavailable, "signing key: %v", err)
	}
	if !signing.PublicKey.Equal(s.cfg.VerifyKey) {
		return nil, nil, errf(http.StatusServiceUnavailable, "the signing key does not match the OpenPak console verification key")
	}
	aesKey, err := os.ReadFile(s.cfg.AESKeyPath)
	if err != nil {
		return nil, nil, errf(http.StatusServiceUnavailable, "AES key unreadable")
	}
	if n := len(aesKey); n != 16 && n != 24 && n != 32 {
		return nil, nil, errf(http.StatusServiceUnavailable, "AES key must be 16, 24 or 32 raw bytes")
	}
	return signing, aesKey, nil
}

// Publish builds, signs, verifies and switches to a new build.
func (s *Service) Publish(note string) (*PublishResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.publish(note)
	if err != nil {
		s.fail("publish", err)
		return nil, err
	}
	return res, nil
}

func (s *Service) fail(op string, err error) {
	s.log.Error("news admin: "+op+" failed", "error", err)
	s.state.LastError = &errInfo{At: s.cfg.Now().UTC(), Op: op, Error: err.Error()}
	if e := s.saveState(); e != nil {
		s.log.Error("news admin: saving state", "error", e)
	}
}

func (s *Service) publish(note string) (*PublishResult, error) {
	signing, aesKey, err := s.loadKeys()
	if err != nil {
		return nil, err
	}
	now := s.cfg.Now().UTC().Truncate(time.Second)
	n := s.state.NextBuild
	dir := s.buildDir(n)
	os.RemoveAll(dir)
	seedDir, signedDir := filepath.Join(dir, "seed"), filepath.Join(dir, "signed")
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(dir)
		}
	}()
	prev := s.newestManifest()
	if prev == nil {
		prev = s.importManifest()
	}
	if err := newsseed.PrepareOutDir(seedDir, prev); err != nil {
		return nil, err
	}
	man, err := newsseed.Build(s.src, seedDir, now, newsseed.Options{AllowEmptyChannels: true})
	if err != nil {
		return nil, errf(http.StatusUnprocessableEntity, "build: %v", err)
	}
	count, err := signTree(seedDir, signedDir, aesKey, s.cfg.SecretIndex, signing, s.cfg.VerifyKey)
	if err != nil {
		return nil, errf(http.StatusInternalServerError, "sign: %v", err)
	}
	h, err := newsexperiment.Handler(signedDir, s.log, s.cfg.Fallback)
	if err != nil {
		return nil, errf(http.StatusInternalServerError, "load signed tree: %v", err)
	}
	// Persist the first-publish stamps, so publication times stay stable.
	if err := s.stampPublished(now); err != nil {
		return nil, err
	}
	articles := 0
	for _, ch := range man.Channels {
		articles += len(ch.Articles)
	}
	changed := changedTopics(s.servedManifest(), man)
	s.state.NextBuild = n + 1
	s.state.Revision++
	s.state.Builds = append(s.state.Builds, buildInfo{Build: n, CreatedAt: now, Containers: count,
		Channels: len(man.Channels), Articles: articles, Note: note})
	s.state.BuildRevs[n] = s.state.SourceRev
	s.state.History = append(s.state.History, n)
	s.state.Serving, s.state.ServingSince = n, now
	s.state.LastPublish = &publishInfo{Build: n, At: now, ChangedTopics: changed, Containers: count, Note: note}
	s.state.LastError = nil
	if err := s.saveState(); err != nil {
		return nil, err
	}
	ok = true
	s.cfg.Switch.Store(h)
	s.log.Info("news admin: published", "build", n, "revision", s.state.Revision, "containers", count, "changed", changed)
	s.prune()
	return &PublishResult{Build: n, Revision: s.state.Revision, PublishedAt: now.Format(time.RFC3339),
		Containers: count, Channels: len(man.Channels), Articles: articles,
		ChangedTopics: changed, Push: s.queuePush(changed, s.state.Revision), Note: note}, nil
}

// stampPublished writes published_at on every article that has none.
func (s *Service) stampPublished(now time.Time) error {
	src, err := s.load()
	if err != nil {
		return err
	}
	stamp := now.Format(time.RFC3339)
	for topic, arts := range src.articles {
		for _, a := range arts {
			if a.PublishedAt == "" {
				a.PublishedAt = stamp
				if err := writeJSON(s.articlePath(topic, a.NewsID), a); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Rollback serves an earlier kept build (default: the one served before).
func (s *Service) Rollback(target int) (*PublishResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.rollback(target)
	if err != nil {
		s.fail("rollback", err)
		return nil, err
	}
	return res, nil
}

func (s *Service) rollback(target int) (*PublishResult, error) {
	hist := s.state.History
	if target == 0 {
		for i := len(hist) - 2; i >= 0; i-- {
			if hist[i] != s.state.Serving && s.kept(hist[i]) != nil {
				target = hist[i]
				break
			}
		}
		if target == 0 {
			return nil, errf(http.StatusConflict, "no earlier kept build to roll back to")
		}
	}
	info := s.kept(target)
	if info == nil {
		return nil, errf(http.StatusConflict, "build %d is not kept", target)
	}
	if target == s.state.Serving {
		return nil, errf(http.StatusConflict, "build %d is already served", target)
	}
	h, err := newsexperiment.Handler(filepath.Join(s.buildDir(target), "signed"), s.log, s.cfg.Fallback)
	if err != nil {
		return nil, errf(http.StatusInternalServerError, "load build %d: %v", target, err)
	}
	var man newsseed.Manifest
	raw, err := os.ReadFile(filepath.Join(s.buildDir(target), "seed", "manifest.json"))
	if err != nil || json.Unmarshal(raw, &man) != nil {
		return nil, errf(http.StatusInternalServerError, "build %d has no readable manifest", target)
	}
	changed := changedTopics(s.servedManifest(), &man)
	now := s.cfg.Now().UTC().Truncate(time.Second)
	// Drop the rolled-back-from tail of the history, so a second rollback
	// keeps going back instead of flipping between two builds.
	for len(s.state.History) > 0 && s.state.History[len(s.state.History)-1] != target {
		s.state.History = s.state.History[:len(s.state.History)-1]
	}
	if len(s.state.History) == 0 {
		s.state.History = []int{target}
	}
	s.state.Revision++
	s.state.Serving, s.state.ServingSince = target, now
	s.state.LastError = nil
	if err := s.saveState(); err != nil {
		return nil, err
	}
	s.cfg.Switch.Store(h)
	s.log.Info("news admin: rolled back", "build", target, "revision", s.state.Revision, "changed", changed)
	articles := 0
	for _, ch := range man.Channels {
		articles += len(ch.Articles)
	}
	return &PublishResult{Build: target, Revision: s.state.Revision, PublishedAt: info.CreatedAt.Format(time.RFC3339),
		Containers: info.Containers, Channels: len(man.Channels), Articles: articles,
		ChangedTopics: changed, Push: s.queuePush(changed, s.state.Revision), Note: info.Note}, nil
}

func (s *Service) kept(n int) *buildInfo {
	for i := range s.state.Builds {
		if s.state.Builds[i].Build == n {
			return &s.state.Builds[i]
		}
	}
	return nil
}

// servedManifest is the manifest of the served build (nil = static tree).
func (s *Service) servedManifest() *newsseed.Manifest {
	if s.state.Serving == 0 {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(s.buildDir(s.state.Serving), "seed", "manifest.json"))
	if err != nil {
		return nil
	}
	var m newsseed.Manifest
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return &m
}

// changedTopics lists every topic whose bcat-list payload differs between
// two builds (nil old = every topic of the new build).
func changedTopics(old, cur *newsseed.Manifest) []string {
	lists := func(m *newsseed.Manifest) map[string]string {
		out := map[string]string{}
		if m == nil {
			return out
		}
		for _, ch := range m.Channels {
			out[ch.TopicID] = ch.Files["list"].SHA256
		}
		for _, ch := range m.Removed {
			if ch.List != nil {
				out[ch.TopicID] = ch.List.SHA256
			}
		}
		return out
	}
	a, b := lists(old), lists(cur)
	out := []string{}
	for topic, sum := range b {
		if a[topic] != sum || old == nil {
			out = append(out, topic)
		}
	}
	sort.Strings(out)
	return out
}

// prune keeps the newest KeepBuilds builds and always the served one.
func (s *Service) prune() {
	var kept []buildInfo
	extra := len(s.state.Builds) - s.cfg.KeepBuilds
	for _, b := range s.state.Builds {
		if extra > 0 && b.Build != s.state.Serving {
			extra--
			os.RemoveAll(s.buildDir(b.Build))
			delete(s.state.BuildRevs, b.Build)
			s.log.Info("news admin: pruned build", "build", b.Build)
			continue
		}
		kept = append(kept, b)
	}
	s.state.Builds = kept
	if err := s.saveState(); err != nil {
		s.log.Error("news admin: saving state", "error", err)
	}
}

// signTree is tools/news-cdn/sign-seed.sh in-process: every *.msgpack and
// icon.jpg except manifest.json becomes a .bcat container, each verified
// independently against the console's public key (size = header +
// payload, exact plaintext, valid signature); live articles also get
// their plaintext local.msgpack; SHA256SUMS lists the tree.
func signTree(in, out string, aesKey []byte, index byte, signing *rsa.PrivateKey, verify *rsa.PublicKey) (int, error) {
	if err := os.MkdirAll(out, 0755); err != nil {
		return 0, err
	}
	var files []string
	err := filepath.WalkDir(in, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && (strings.HasSuffix(d.Name(), ".msgpack") || d.Name() == "icon.jpg") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	sort.Strings(files)
	count := 0
	for _, path := range files {
		rel, _ := filepath.Rel(in, path)
		rel = filepath.ToSlash(rel)
		plain, err := os.ReadFile(path)
		if err != nil {
			return count, err
		}
		dst := strings.TrimSuffix(strings.TrimSuffix(rel, ".msgpack"), ".jpg") + ".bcat"
		container, err := bcatcontainer.Encode(plain, aesKey, index, signing)
		if err != nil {
			return count, fmt.Errorf("%s: %w", rel, err)
		}
		if len(container) != bcatcontainer.HeaderSize+len(plain) {
			return count, fmt.Errorf("%s: size mismatch (lists advertise 288 + payload bytes)", rel)
		}
		got, err := bcatcontainer.Verify(container, aesKey, verify)
		if err != nil {
			return count, fmt.Errorf("%s: verification failed: %w", rel, err)
		}
		if !bytes.Equal(got, plain) {
			return count, fmt.Errorf("%s: verification failed: plaintext differs", rel)
		}
		if err := writeFileMkdir(filepath.Join(out, dst), container); err != nil {
			return count, err
		}
		if strings.HasSuffix(rel, "/article.msgpack") && !strings.HasPrefix(rel, "retired/") {
			local := strings.TrimSuffix(rel, "article.msgpack") + "local.msgpack"
			if err := writeFileMkdir(filepath.Join(out, local), plain); err != nil {
				return count, err
			}
		}
		count++
	}
	return count, writeSums(out)
}

func writeFileMkdir(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// writeSums writes a sha256sum -c compatible SHA256SUMS (self-excluded).
func writeSums(dir string) error {
	var lines []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() || d.Name() == "SHA256SUMS" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		sum := sha256.Sum256(data)
		lines = append(lines, hex.EncodeToString(sum[:])+"  ./"+filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i][66:] < lines[j][66:] })
	return os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

// ---------- push ----------

// Pusher asks nx-baas to notify consoles subscribed to a topic.
type Pusher struct {
	URL       string // NX_BAAS_PUSH_URL, e.g. http://openpak-nx-baas:20070/internal/news/push
	Key       string // NX_BAAS_INTERNAL_KEY
	WaitRange int    // optional wait_range seconds; 0 = nx-baas/console default
	Client    *http.Client
}

func (p *Pusher) push(ctx context.Context, topic string, revision int64) PushResult {
	res := PushResult{TopicID: topic, Revision: revision, At: time.Now().UTC()}
	body := map[string]any{"topic_id": topic, "revision": revision}
	if p.WaitRange > 0 {
		body["wait_range"] = p.WaitRange
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, bytes.NewReader(raw))
	if err != nil {
		res.Error = err.Error()
		return res
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", p.Key)
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	resp.Body.Close()
	res.Status = resp.StatusCode
	res.OK = resp.StatusCode/100 == 2
	if !res.OK {
		res.Error = resp.Status
	}
	return res
}

// queuePush pushes every changed topic in the background; failures are
// logged and recorded, never fatal. Caller holds s.mu.
func (s *Service) queuePush(topics []string, revision int64) string {
	if s.cfg.Push == nil || s.cfg.Push.URL == "" {
		return "disabled"
	}
	if len(topics) == 0 {
		return "none"
	}
	s.push.Add(1)
	go func() {
		defer s.push.Done()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		var results []PushResult
		var failed error
		for _, t := range topics {
			r := s.cfg.Push.push(ctx, t, revision)
			if r.OK {
				s.log.Info("news admin: pushed", "topic", t, "revision", revision)
			} else {
				s.log.Warn("news admin: push failed", "topic", t, "revision", revision, "error", r.Error)
				failed = errors.Join(failed, fmt.Errorf("%s: %s", t, r.Error))
			}
			results = append(results, r)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.state.Pushes = results
		if failed != nil {
			s.state.LastError = &errInfo{At: s.cfg.Now().UTC(), Op: "push", Error: failed.Error()}
		}
		if err := s.saveState(); err != nil {
			s.log.Error("news admin: saving state", "error", err)
		}
	}()
	return "queued"
}

// WaitPushes blocks until background pushes finish (tests, shutdown).
func (s *Service) WaitPushes() { s.push.Wait() }
