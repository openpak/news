#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Sign a newsseed build into BCAT containers for the experimental CDN.
#
# Usage:
#   tools/news-cdn/sign-seed.sh -i BUILD_DIR -o SIGNED_DIR \
#     -k SIGNING_PEM -a AES_KEY_FILE [-x SECRET_INDEX] [-f]
#
# - BUILD_DIR is a cmd/newsseed output directory (decoded payloads).
# - SIGNED_DIR mirrors the tree with .bcat containers; must not exist
#   unless -f is given (updates remove it first).
# - The signing key and AES key stay on the operator's machine; never
#   commit, copy or print them. The CDN image receives containers only.
# - The retired/ tree and manifest.json are not signed: retired fixtures
#   are prepared cleanup artifacts, not CDN routes.
#
# Every produced container is verified against the public OpenPak key
# (tools/news-patch/openpak-bcat-public.pem) before the script reports
# success, using openssl only.
set -euo pipefail

in_dir="" out_dir="" signing="" aes="" index=0 force=0
while getopts "i:o:k:a:x:f" opt; do
  case "$opt" in
    i) in_dir=$OPTARG ;;
    o) out_dir=$OPTARG ;;
    k) signing=$OPTARG ;;
    a) aes=$OPTARG ;;
    x) index=$OPTARG ;;
    f) force=1 ;;
    *) echo "usage: $0 -i BUILD_DIR -o SIGNED_DIR -k SIGNING_PEM -a AES_KEY_FILE [-x N] [-f]" >&2; exit 2 ;;
  esac
done
[[ -n "$in_dir" && -n "$out_dir" && -n "$signing" && -n "$aes" ]] || {
  echo "usage: $0 -i BUILD_DIR -o SIGNED_DIR -k SIGNING_PEM -a AES_KEY_FILE [-x N] [-f]" >&2; exit 2; }
[[ -d "$in_dir" ]] || { echo "no build dir: $in_dir" >&2; exit 1; }
[[ -f "$signing" && -f "$aes" ]] || { echo "key files missing" >&2; exit 1; }
if [[ -e "$out_dir" ]]; then
  if [[ "$force" == 1 ]]; then rm -rf "$out_dir"; else
    echo "output exists: $out_dir (use -f to replace)" >&2; exit 1; fi
fi

repo="$(cd "$(dirname "$0")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
echo "building newscontainer..."
(cd "$repo" && CGO_ENABLED=0 go build -trimpath -o "$tmp/newscontainer" ./cmd/newscontainer)

pub="$repo/tools/news-patch/openpak-bcat-public.pem"
[[ -f "$pub" ]] || { echo "public verification key missing: $pub" >&2; exit 1; }

count=0
fail=0
while IFS= read -r src; do
  rel="${src#"$in_dir"/}"
  case "$rel" in
    retired/*) continue ;;
    manifest.json) continue ;;
  esac
  dst="$out_dir/${rel%.msgpack}.bcat"
  [[ "$src" == *.jpg ]] && dst="$out_dir/${rel%.jpg}.bcat"
  mkdir -p "$(dirname "$dst")"
  "$tmp/newscontainer" -in "$src" -out "$dst" \
    -aes-key-file "$aes" -signing-key-file "$signing" -secret-index "$index" > /dev/null

  # Independent verification with the public key: decrypt the payload
  # (AES-256-CTR, IV from the header) and check the RSA/SHA-256 signature
  # over the plaintext.
  keyhex="$(od -An -tx1 "$aes" | tr -d ' \n')"
  ivhex="$(od -An -tx1 -N16 -j16 "$dst" | tr -d ' \n')"
  payload="$tmp/plain.bin"
  size=$(( $(stat -c%s "$dst") - 288 ))
  dd if="$dst" of="$payload" bs=1 skip=288 count="$size" status=none
  openssl enc -d -aes-256-ctr -K "$keyhex" -iv "$ivhex" -in "$payload" -out "$payload.dec" -nopad
  sig="$tmp/sig.bin"
  dd if="$dst" of="$sig" bs=1 skip=32 count=256 status=none
  if openssl dgst -sha256 -verify "$pub" -signature "$sig" "$payload.dec" > /dev/null 2>&1; then
    echo "  signed+verified  $rel"
    count=$((count + 1))
  else
    echo "  VERIFY FAILED    $rel" >&2
    fail=1
  fi
done < <(find "$in_dir" -type f \( -name '*.msgpack' -o -name 'icon.jpg' \) | sort)

if [[ "$fail" == 1 ]]; then
  echo "verification failed; $out_dir is incomplete" >&2
  exit 1
fi
echo "$count containers signed and verified in $out_dir"
( cd "$out_dir" && find . -type f -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS )
echo "checksums: $out_dir/SHA256SUMS"
