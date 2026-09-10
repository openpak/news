// SPDX-License-Identifier: AGPL-3.0-only
// Package bcatcontainer encodes experimental containers for the OpenPak-key
// patched BCAT module. It contains no platform encryption secrets.
package bcatcontainer

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"io"
)

const HeaderSize = 0x120
const MaxPayload = 4 << 20

// Encode signs the plaintext with RSA PKCS#1 v1.5/SHA-256, then encrypts it
// with AES-CTR. aesKey must already have been derived for the target title
// and secret index. The IV starts at payload offset zero, excluding the header.
func Encode(plaintext, aesKey []byte, secretIndex byte, signingKey *rsa.PrivateKey) ([]byte, error) {
	if len(plaintext) == 0 || len(plaintext) > MaxPayload {
		return nil, errors.New("payload size outside supported range")
	}
	if secretIndex > 15 {
		return nil, errors.New("secret index must be 0..15")
	}
	if signingKey == nil || signingKey.N == nil || signingKey.N.BitLen() != 2048 || signingKey.E != 65537 {
		return nil, errors.New("need RSA-2048/65537 signing key")
	}
	if err := signingKey.Validate(); err != nil {
		return nil, errors.New("invalid RSA signing key")
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}
	out := make([]byte, HeaderSize+len(plaintext))
	copy(out, "bcat")
	out[4], out[5], out[6], out[7] = 1, byte((len(aesKey)-16)/8+1), 1, secretIndex
	if _, err := io.ReadFull(rand.Reader, out[0x10:0x20]); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(plaintext)
	signature, err := rsa.SignPKCS1v15(rand.Reader, signingKey, crypto.SHA256, digest[:])
	if err != nil {
		return nil, err
	}
	copy(out[0x20:HeaderSize], signature)
	cipher.NewCTR(block, out[0x10:0x20]).XORKeyStream(out[HeaderSize:], plaintext)
	return out, nil
}
