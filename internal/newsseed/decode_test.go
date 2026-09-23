// SPDX-License-Identifier: AGPL-3.0-only
package newsseed

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openpak/news/internal/newsformat"
)

// decode is a test-only MessagePack reader for the subset newsformat
// writes. Maps come back as newsformat.Map so key order can be asserted.
func decode(b []byte) (any, []byte, error) {
	if len(b) == 0 {
		return nil, nil, fmt.Errorf("short input")
	}
	tag, rest := b[0], b[1:]
	n := func(size int) (uint64, []byte, error) {
		if len(rest) < size {
			return 0, nil, fmt.Errorf("short length")
		}
		var v uint64
		for _, c := range rest[:size] {
			v = v<<8 | uint64(c)
		}
		return v, rest[size:], nil
	}
	str := func(l uint64, r []byte) (any, []byte, error) {
		if uint64(len(r)) < l {
			return nil, nil, fmt.Errorf("short string")
		}
		return string(r[:l]), r[l:], nil
	}
	arr := func(l uint64, r []byte) (any, []byte, error) {
		out := []any{}
		for i := uint64(0); i < l; i++ {
			v, next, err := decode(r)
			if err != nil {
				return nil, nil, err
			}
			out, r = append(out, v), next
		}
		return out, r, nil
	}
	mp := func(l uint64, r []byte) (any, []byte, error) {
		out := newsformat.Map{}
		for i := uint64(0); i < l; i++ {
			k, next, err := decode(r)
			if err != nil {
				return nil, nil, err
			}
			v, next, err := decode(next)
			if err != nil {
				return nil, nil, err
			}
			out, r = append(out, newsformat.Field{Key: k.(string), Value: v}), next
		}
		return out, r, nil
	}
	switch {
	case tag <= 0x7f:
		return int64(tag), rest, nil
	case tag >= 0xe0:
		return int64(int8(tag)), rest, nil
	case tag&0xe0 == 0xa0:
		return str(uint64(tag&0x1f), rest)
	case tag&0xf0 == 0x90:
		return arr(uint64(tag&0x0f), rest)
	case tag&0xf0 == 0x80:
		return mp(uint64(tag&0x0f), rest)
	}
	switch tag {
	case 0xc2:
		return false, rest, nil
	case 0xc3:
		return true, rest, nil
	case 0xcc, 0xcd, 0xce, 0xcf:
		v, r, err := n(1 << (tag - 0xcc))
		return int64(v), r, err
	case 0xd0, 0xd1, 0xd2, 0xd3:
		size := 1 << (tag - 0xd0)
		v, r, err := n(size)
		shift := 64 - 8*size
		return int64(v<<shift) >> shift, r, err
	case 0xd9, 0xda, 0xdb:
		l, r, err := n(1 << (tag - 0xd9))
		if err != nil {
			return nil, nil, err
		}
		return str(l, r)
	case 0xc4, 0xc5, 0xc6:
		l, r, err := n(1 << (tag - 0xc4))
		if err != nil || uint64(len(r)) < l {
			return nil, nil, fmt.Errorf("short bin")
		}
		return r[:l], r[l:], nil
	case 0xdc, 0xdd:
		l, r, err := n(2 << (tag - 0xdc))
		if err != nil {
			return nil, nil, err
		}
		return arr(l, r)
	case 0xde, 0xdf:
		l, r, err := n(2 << (tag - 0xde))
		if err != nil {
			return nil, nil, err
		}
		return mp(l, r)
	}
	return nil, nil, fmt.Errorf("unsupported tag %#x", tag)
}

func decodeFile(t *testing.T, path string) any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	v, rest, err := decode(raw)
	if err != nil || len(rest) != 0 {
		t.Fatalf("%s: decode: %v (%d trailing bytes)", filepath.Base(path), err, len(rest))
	}
	return v
}

func keys(m newsformat.Map) []string {
	out := make([]string, 0, len(m))
	for _, f := range m {
		out = append(out, f.Key)
	}
	return out
}

func get(m newsformat.Map, key string) any {
	for _, f := range m {
		if f.Key == key {
			return f.Value
		}
	}
	return nil
}
