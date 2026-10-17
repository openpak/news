// SPDX-License-Identifier: AGPL-3.0-only

// Package datasetfiles loads operator-supplied BCAT datasets into an immutable
// snapshot. It uses OpenPak's emulator manifest contract, not a game format.
package datasetfiles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"time"

	"github.com/openpak/news/internal/content"
)

const maxFileSize = 64 << 20
const maxSnapshotSize = 128 << 20

var bcatPath = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,31}/[A-Za-z0-9_.-]{1,31}$`)

type manifest struct {
	Format     int        `json:"format"`
	TitleID    string     `json:"title_id"`
	ValidFrom  time.Time  `json:"valid_from"`
	ValidUntil *time.Time `json:"valid_until"`
	Files      []struct {
		Path   string `json:"path"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

type dataset struct {
	value content.Dataset
	mod   time.Time
}

type Source struct {
	datasets map[string]dataset
	objects  map[string][]byte
	mod      time.Time
}

// Load validates every manifest and file before making any dataset available.
// root/<title>/manifest.json describes root/<title>/files/<directory>/<file>.
// Bytes are retained in memory so changing a mounted file cannot invalidate a
// served manifest's hashes. Replacing a dataset requires restarting the service.
func Load(path string) (*Source, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	s := &Source{datasets: map[string]dataset{}, objects: map[string][]byte{}}
	var total int64
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		title := entry.Name()
		if err := content.ValidateSwitchTitleID(title); err != nil {
			return nil, err
		}
		body, mod, err := read(root, title+"/manifest.json", 1<<20)
		if err != nil {
			return nil, fmt.Errorf("%s manifest: %w", title, err)
		}
		var m manifest
		if err := json.Unmarshal(body, &m); err != nil {
			return nil, fmt.Errorf("%s manifest: %w", title, err)
		}
		if m.Format != 1 || m.TitleID != title || m.ValidFrom.IsZero() ||
			m.ValidUntil == nil || !m.ValidUntil.After(m.ValidFrom) || len(m.Files) == 0 {
			return nil, fmt.Errorf("%s: require format 1, matching title, finite validity window and files", title)
		}
		d := content.Dataset{ID: title, TitleID: title, ValidFrom: m.ValidFrom,
			ValidUntil: m.ValidUntil, Sealed: true}
		for _, f := range m.Files {
			if content.ValidateFilePath(f.Path) != nil || !bcatPath.MatchString(f.Path) ||
				f.Size < 0 || f.Size > maxFileSize || total+f.Size > maxSnapshotSize {
				return nil, fmt.Errorf("%s: invalid path or size for %q", title, f.Path)
			}
			key := title + "/files/" + f.Path
			if _, duplicate := s.objects[key]; duplicate {
				return nil, fmt.Errorf("%s: duplicate path %q", title, f.Path)
			}
			data, fileMod, err := read(root, key, f.Size)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			hash := sha256.Sum256(data)
			if int64(len(data)) != f.Size || hex.EncodeToString(hash[:]) != f.SHA256 {
				return nil, fmt.Errorf("%s: size or SHA-256 mismatch", key)
			}
			total += f.Size
			s.objects[key] = data
			d.Files = append(d.Files, content.DatasetFile{Path: f.Path, Key: key, Size: f.Size, SHA256: f.SHA256})
			if fileMod.After(mod) {
				mod = fileMod
			}
		}
		s.datasets[title] = dataset{d, mod}
		if mod.After(s.mod) {
			s.mod = mod
		}
	}
	return s, nil
}

func read(root *os.Root, path string, limit int64) ([]byte, time.Time, error) {
	f, err := root.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, time.Time{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, time.Time{}, fmt.Errorf("not a regular file within size limit")
	}
	body, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(body)) > limit {
		return nil, time.Time{}, fmt.Errorf("file exceeds size limit")
	}
	return body, info.ModTime(), err
}

func (s *Source) Titles(ctx context.Context) ([]string, time.Time, error) {
	titles := []string{}
	for title := range s.datasets {
		if _, _, err := s.CurrentDataset(ctx, title); err == nil {
			titles = append(titles, title)
		}
	}
	sort.Strings(titles)
	return titles, s.mod, nil
}

func (s *Source) CurrentDataset(_ context.Context, title string) (*content.Dataset, time.Time, error) {
	d, ok := s.datasets[title]
	now := time.Now()
	if !ok || now.Before(d.value.ValidFrom) || !now.Before(*d.value.ValidUntil) {
		return nil, time.Time{}, content.ErrNotFound
	}
	return &d.value, d.mod, nil
}

type object struct{ *bytes.Reader }

func (object) Close() error { return nil }

func (s *Source) OpenObject(_ context.Context, key string) (io.ReadSeekCloser, int64, time.Time, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, 0, time.Time{}, content.ErrNotFound
	}
	return object{bytes.NewReader(data)}, int64(len(data)), s.mod, nil
}

func (*Source) Articles(context.Context, content.AudienceQuery) ([]content.Article, time.Time, error) {
	return nil, time.Time{}, nil
}

func (*Source) ImageByKey(context.Context, string) (*content.ArticleImage, time.Time, error) {
	return nil, time.Time{}, content.ErrNotFound
}
