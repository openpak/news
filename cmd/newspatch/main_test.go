// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

func TestNewsAuthPatchScopeAndGuards(t *testing.T) {
	nso := make([]byte, 0x800)
	copy(nso, "NSO0")
	nso[0x40] = 42
	copy(nso[0x400:], []byte{0xe1, 0x03, 0x14, 0xaa, 0x46, 0x09, 0x00, 0x94})
	p := profile{build: hex.EncodeToString(nso[0x40:0x60]), sourceHash: digest(nso)}
	patch, err := makeNewsAuthPatch(nso, p, 0x400)
	if err != nil {
		t.Fatal(err)
	}
	if len(patch) != 21 || string(patch[:5]) != "PATCH" || string(patch[18:]) != "EOF" {
		t.Fatal("bad IPS framing")
	}
	off := int(patch[5])<<16 | int(patch[6])<<8 | int(patch[7])
	n := int(patch[8])<<8 | int(patch[9])
	if off != 0x400 || n != 8 {
		t.Fatal("wrong patch range")
	}
	patched := bytes.Clone(nso)
	copy(patched[off:off+n], patch[10:18])
	if !bytes.Equal(patched[:off], nso[:off]) || !bytes.Equal(patched[off+n:], nso[off+n:]) {
		t.Fatal("patch escaped call site")
	}
	for _, offset := range []int{-1, 0, 0x7ff, 0xffffff, 0x404} {
		if _, err := makeNewsAuthPatch(nso, p, offset); err == nil {
			t.Fatalf("accepted offset %x", offset)
		}
	}
	for _, offset := range []int{0, 0x40, 0x600} {
		changed := bytes.Clone(nso)
		changed[offset] ^= 1
		if _, err := makeNewsAuthPatch(changed, p, 0x400); err == nil {
			t.Fatal("accepted modified NSO")
		}
	}
	changed := bytes.Clone(nso)
	changed[0x400] ^= 1
	p.sourceHash = digest(changed)
	if _, err := makeNewsAuthPatch(changed, p, 0x400); err == nil {
		t.Fatal("accepted changed call site")
	}
	path := filepath.Join(t.TempDir(), "patch.ips")
	if err := writePatch(path, patch); err != nil {
		t.Fatal(err)
	}
	if err := writePatch(path, []byte("overwrite")); err == nil {
		t.Fatal("overwrote existing patch")
	}
	stored, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(stored, patch) {
		t.Fatal("existing patch changed")
	}
}

func TestPatchTrustAndScope(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	original := make([]byte, 0x800)
	copy(original, "NSO0")
	original[0x40] = 42
	p := profile{build: hex.EncodeToString(original[0x40:0x60]), sourceHash: digest(original), modulusHash: digest(original[0x400:0x501]), offset: 0x400}
	patch, err := makePatch(original, &key.PublicKey, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(patch) != 270 || string(patch[:5]) != "PATCH" || string(patch[267:]) != "EOF" {
		t.Fatal("bad IPS framing")
	}
	off := int(patch[5])<<16 | int(patch[6])<<8 | int(patch[7])
	n := int(patch[8])<<8 | int(patch[9])
	patched := bytes.Clone(original)
	copy(patched[off:off+n], patch[10:10+n])
	if off != 0x400 || n != 257 || !bytes.Equal(original[:off], patched[:off]) || !bytes.Equal(original[off+n:], patched[off+n:]) {
		t.Fatal("patch escaped modulus")
	}
	public := rsa.PublicKey{N: new(big.Int).SetBytes(patched[off : off+n]), E: 65537}
	hash := sha256.Sum256([]byte("original OpenPak catalog"))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	if err = rsa.VerifyPKCS1v15(&public, crypto.SHA256, hash[:], sig); err != nil {
		t.Fatal(err)
	}
	sig[0] ^= 1
	if rsa.VerifyPKCS1v15(&public, crypto.SHA256, hash[:], sig) == nil {
		t.Fatal("tampered signature accepted")
	}
	changed := bytes.Clone(original)
	changed[0x600] ^= 1
	if _, err = makePatch(changed, &key.PublicKey, p); err == nil {
		t.Fatal("modified executable accepted")
	}
	changed = bytes.Clone(original)
	changed[0x40] ^= 1
	if _, err = makePatch(changed, &key.PublicKey, p); err == nil {
		t.Fatal("wrong build accepted")
	}
	wrong := key.PublicKey
	wrong.E = 3
	if _, err = makePatch(original, &wrong, p); err == nil {
		t.Fatal("wrong exponent accepted")
	}
}
