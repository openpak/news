// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
)

// MessagePack wire vectors: type tags, byte lengths (including UTF-8),
// signed integers, and stable key ordering. Checked against the format.
func TestPackWireVectors(t *testing.T) {
	for _, tc := range []struct {
		value any
		wire  string
	}{
		{false, "c2"}, {true, "c3"},
		{int64(-1), "ff"},
		{int64(1), "01"},
		{int64(127), "7f"}, {int64(128), "cc80"},
		{int64(255), "ccff"}, {int64(256), "cd0100"},
		{int64(65535), "cdffff"}, {int64(65536), "ce00010000"},
		{int64(4294967295), "ceffffffff"}, {int64(4294967296), "cf0000000100000000"},
		{int64(-32), "e0"}, {int64(-33), "d0df"},
		{int64(-128), "d080"}, {int64(-129), "d1ff7f"},
		{int64(-32769), "d2ffff7fff"}, {int64(-2147483649), "d3ffffffff7fffffff"},
		{"é", "a2c3a9"},
		{[]byte{0, 255}, "c40200ff"},
		{[]any{}, "90"},
		{[]any{"x"}, "91a178"},
		{map[string]any{"b": "", "a": ""}, "82a161a0a162a0"},
		{orderedMap{{"b", ""}, {"a", ""}}, "82a162a0a161a0"},
	} {
		var out bytes.Buffer
		if err := pack(&out, tc.value); err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(out.Bytes()); got != tc.wire {
			t.Fatalf("%T: got %s, want %s", tc.value, got, tc.wire)
		}
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestPackRejectsUnsupportedAndPropagatesFailure(t *testing.T) {
	if err := pack(&bytes.Buffer{}, struct{}{}); err == nil {
		t.Fatal("unsupported type accepted")
	}
	if err := pack(failedWriter{}, map[string]any{"x": int64(1)}); err == nil {
		t.Fatal("write error lost")
	}
}

type shortWriter struct{}

func (shortWriter) Write(b []byte) (int, error) { return len(b) - 1, nil }
func TestShortWrite(t *testing.T) {
	if err := pack(shortWriter{}, int64(256)); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("got %v", err)
	}
}
func TestLengthBoundaries(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want string
	}{
		{31, "bf"}, {32, "d920"}, {255, "d9ff"}, {256, "da0100"}, {65535, "daffff"}, {65536, "db00010000"},
	} {
		var b bytes.Buffer
		if err := pack(&b, strings.Repeat("a", tc.n)); err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(b.Bytes()[:len(tc.want)/2]); got != tc.want {
			t.Fatalf("length %d: %s", tc.n, got)
		}
	}
}
