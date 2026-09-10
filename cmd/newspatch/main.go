// SPDX-License-Identifier: AGPL-3.0-only
// newspatch generates separately installable patches for the pinned BCAT build.
package main

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
)

type profile struct {
	build, sourceHash, modulusHash string
	offset                         int
}

var firmware225 = profile{
	build:       "6d9772a733e2370b3f401efeb7ca8f664309bd66000000000000000000000000",
	sourceHash:  "dfe4c5361e91db1c508dedc6289fdd04d86f7990f4be6357f245649090733b5a",
	modulusHash: "57e84b18abae9ccf7b066bd8048fb28b9e353bf7421b7657665ecd43d3a44e1d",
	offset:      0x220c24, // uncompressed NSO: 0x100 header + mapped address 0x220b24
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// This call site belongs to the News memory-download worker, before curl is
// started. Do not patch the shared token helper: other BCAT clients use it.
func makeNewsAuthPatch(nso []byte, p profile, offset int) ([]byte, error) {
	if len(nso) < 0x100 || !bytes.Equal(nso[:4], []byte("NSO0")) ||
		hex.EncodeToString(nso[0x40:0x60]) != p.build || digest(nso) != p.sourceHash {
		return nil, fmt.Errorf("unsupported or modified BCAT executable")
	}
	old := []byte{0xe1, 0x03, 0x14, 0xaa, 0x46, 0x09, 0x00, 0x94}
	if offset < 0x100 || offset > 0xffffff || offset+len(old) > len(nso) ||
		!bytes.Equal(nso[offset:offset+len(old)], old) {
		return nil, fmt.Errorf("unexpected News token call site")
	}
	// strb wzr,[x21]; mov w0,wzr. Explicitly terminate the output token and
	// report success without calling dauth. The surrounding lock/state cleanup
	// and subsequent HTTP/container verification paths remain intact.
	replacement := []byte{0xbf, 0x02, 0x00, 0x39, 0xe0, 0x03, 0x1f, 0x2a}
	out := []byte("PATCH")
	out = append(out, byte(offset>>16), byte(offset>>8), byte(offset), 0, 8)
	out = append(out, replacement...)
	return append(out, []byte("EOF")...), nil
}
func makePatch(nso []byte, key *rsa.PublicKey, p profile) ([]byte, error) {
	if len(nso) < 0x100 || !bytes.Equal(nso[:4], []byte("NSO0")) {
		return nil, fmt.Errorf("expected uncompressed NSO0")
	}
	if hex.EncodeToString(nso[0x40:0x60]) != p.build || digest(nso) != p.sourceHash {
		return nil, fmt.Errorf("unsupported or modified BCAT executable")
	}
	if p.offset < 0x100 || p.offset+257 > len(nso) || p.offset > 0xffffff {
		return nil, fmt.Errorf("invalid patch range")
	}
	old := nso[p.offset : p.offset+257]
	if digest(old) != p.modulusHash {
		return nil, fmt.Errorf("original modulus mismatch")
	}
	if key == nil || key.N == nil || key.N.Sign() <= 0 || key.N.BitLen() != 2048 || key.E != 65537 {
		return nil, fmt.Errorf("need RSA-2048 public key with exponent 65537")
	}
	replacement := make([]byte, 257)
	key.N.FillBytes(replacement[1:])
	if bytes.Equal(old, replacement) {
		return nil, fmt.Errorf("replacement key is unchanged")
	}
	out := []byte("PATCH")
	out = append(out, byte(p.offset>>16), byte(p.offset>>8), byte(p.offset), 1, 1)
	out = append(out, replacement...)
	return append(out, []byte("EOF")...), nil
}
func run() error {
	input := flag.String("nso", "", "owner-provided uncompressed BCAT NSO (private)")
	pub := flag.String("public-key", "", "OpenPak RSA public key in PEM format")
	output := flag.String("out", "", "output IPS path (must not exist)")
	newsNoDauth := flag.Bool("news-no-dauth", false, "experimental News memory-download token omission patch (no public key needed)")
	flag.Parse()
	if *input == "" || *output == "" || (!*newsNoDauth && *pub == "") || (*newsNoDauth && *pub != "") {
		return fmt.Errorf("require -nso, -out and either -public-key or -news-no-dauth")
	}
	nso, err := os.ReadFile(*input)
	if err != nil {
		return err
	}
	var patch []byte
	if *newsNoDauth {
		patch, err = makeNewsAuthPatch(nso, firmware225, 0xfc8a4) // mapped 0xfc7a4 + NSO header
	} else {
		patch, err = makeKeyPatchFromPEM(nso, *pub)
	}
	if err != nil {
		return err
	}
	return writePatch(*output, patch)
}

func makeKeyPatchFromPEM(nso []byte, publicPath string) ([]byte, error) {
	encoded, err := os.ReadFile(publicPath)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(encoded)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("expected one PKIX PUBLIC KEY PEM block")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA")
	}
	return makePatch(nso, key, firmware225)
}

func writePatch(output string, patch []byte) error {
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	if _, err = f.Write(patch); err != nil {
		f.Close()
		os.Remove(output)
		return err
	}
	if err = f.Close(); err != nil {
		os.Remove(output)
		return err
	}
	fmt.Printf("Created %s: %d bytes, SHA-256 %s\n", output, len(patch), digest(patch))
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
