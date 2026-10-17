// SPDX-License-Identifier: AGPL-3.0-only
package newsadmin

import (
	"archive/tar"
	"context"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// BcatSync is the nx-bcat hand-off. With it set, a publish builds the unsigned seed and tells
// nx-bcat to pull it (POST URL, X-Internal-Key Key): signing, the console tree and the topic
// push are nx-bcat's, so this process needs no signing key and serves no console tree.
type BcatSync struct {
	URL string // NX_BCAT_SYNC_URL, e.g. http://openpak-nx-bcat:20070/internal/bcat/sync
	Key string // NX_INTERNAL_KEY
}

// queueSync asks nx-bcat to pull the build now served; failures are logged and recorded (the
// nx-bcat poll catches up), never fatal. Caller holds s.mu.
func (s *Service) queueSync() string {
	s.push.Add(1)
	go func() {
		defer s.push.Done()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Bcat.URL, nil)
		var resp *http.Response
		if err == nil {
			req.Header.Set("X-Internal-Key", s.cfg.Bcat.Key)
			resp, err = http.DefaultClient.Do(req)
		}
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				err = &statusError{resp.Status}
			}
		}
		if err == nil {
			s.log.Info("news admin: nx-bcat synced")
			return
		}
		s.log.Warn("news admin: nx-bcat sync failed", "error", err)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.state.LastError = &errInfo{At: s.cfg.Now().UTC(), Op: "bcat-sync", Error: err.Error()}
		if e := s.saveState(); e != nil {
			s.log.Error("news admin: saving state", "error", e)
		}
	}()
	return "delegated"
}

type statusError struct{ status string }

func (e *statusError) Error() string { return e.status }

// bcatCurrent is GET /admin/v1/bcat/current: the build nx-bcat should serve. Build 0 = none
// published yet. The revision moves on every publish and rollback.
func (s *Service) bcatCurrent() map[string]any {
	return map[string]any{"build": s.state.Serving, "revision": s.state.Revision}
}

// writeSeedTar is GET /admin/v1/bcat/builds/{n}/seed.tar: the files nx-bcat signs (the same set
// signTree takes) plus manifest.json, which it diffs to know which topics changed.
func (s *Service) writeSeedTar(w http.ResponseWriter, nstr string) error {
	n, err := strconv.Atoi(nstr)
	if err != nil || s.kept(n) == nil {
		return errf(http.StatusNotFound, "build %q is not kept", nstr)
	}
	seed := filepath.Join(s.buildDir(n), "seed")
	w.Header().Set("Content-Type", "application/x-tar")
	tw := tar.NewWriter(w)
	defer tw.Close()
	return filepath.WalkDir(seed, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		if name := d.Name(); !(strings.HasSuffix(name, ".msgpack") || name == "icon.jpg" || name == "manifest.json") {
			return nil
		}
		rel, _ := filepath.Rel(seed, path)
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0644, Size: int64(len(b)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err = tw.Write(b)
		return err
	})
}
