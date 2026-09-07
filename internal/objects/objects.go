// Package objects is the object store for news images and BCAT dataset
// files. Keys are content hashes, so traversal and overwrite are impossible
// by construction.
package objects

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrNotFound = errors.New("object not found")

type Info struct {
	Key     string
	Size    int64
	SHA256  string
	ModTime time.Time
}

// Store persists immutable content-addressed blobs.
type Store interface {
	Put(ctx context.Context, r io.Reader) (Info, error)
	Open(ctx context.Context, key string) (io.ReadSeekCloser, Info, error)
}

// FS is a filesystem-backed Store. Keys are "<sha256 hex><ext>"; the
// extension is taken from the uploaded file name (validated by callers) and
// is metadata only — content addressing is the hash.
type FS struct{ root string }

func NewFS(root string) (*FS, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &FS{root: root}, nil
}

func (s *FS) path(key string) (string, error) {
	if strings.ContainsAny(key, "/\\") || strings.Contains(key, "..") || key == "" {
		return "", fmt.Errorf("objects: bad key %q", key)
	}
	return filepath.Join(s.root, key), nil
}

func (s *FS) Put(ctx context.Context, r io.Reader) (Info, error) {
	tmp, err := os.CreateTemp(s.root, ".upload-*")
	if err != nil {
		return Info{}, err
	}
	defer os.Remove(tmp.Name())

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	if err != nil {
		tmp.Close()
		return Info{}, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return Info{}, err
	}
	info := Info{Size: n, SHA256: hex.EncodeToString(h.Sum(nil)), ModTime: time.Now().UTC()}
	if f, ok := r.(interface{ Extension() string }); ok {
		info.Key = info.SHA256 + f.Extension()
	} else {
		info.Key = info.SHA256
	}
	dst, err := s.path(info.Key)
	if err != nil {
		tmp.Close()
		return Info{}, err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		tmp.Close()
		return Info{}, err
	}
	return info, tmp.Close()
}

func (s *FS) Open(ctx context.Context, key string) (io.ReadSeekCloser, Info, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, Info{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, Info{}, ErrNotFound
		}
		return nil, Info{}, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, Info{}, err
	}
	return f, Info{Key: key, Size: st.Size(), ModTime: st.ModTime().UTC()}, nil
}
