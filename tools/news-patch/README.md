# Experimental OpenPak BCAT verification-key patch

For the owner's HOS 22.5.0 BCAT module (`010000000000000c`) only:

`6D9772A733E2370B3F401EFEB7CA8F664309BD66000000000000000000000000`

The 270-byte IPS replaces the 257-byte retail RSA-2048 modulus with
`openpak-bcat-public.pem`. Verification instructions, exponent and AES
parameters are unchanged. This lets the verifier use OpenPak signatures;
it is not a signature-check bypass. With this key patch and the separate
[News token patch](news-no-dauth/README.md), probe v0.19 successfully downloaded
and decoded the 155-byte catalog on hardware. Channel subscriptions and
server-delivered articles still need validation.

This is the shared **retail BCAT key**, affecting game BCAT as well as News.
Nintendo-signed downloads using that key will no longer verify while patched.
It does not remove existing articles. The server still needs correctly signed
and encrypted containers; its JSON catalog response is not such a container.
Catalog/detail decoded schemas also need hardware validation.

## Installation and rollback

Preferred: use [probe v0.19](../news-probe/README.md), **R** to install,
**L** to remove. The installer checks the running module build before writing.

Manual installation: place the IPS in
`sd:/atmosphere/exefs_patches/openpak_bcat_key/`, preserving its filename.
Reboot Atmosphere. To roll back, delete this IPS and reboot. If a boot fails,
remove the file using another SD reader. The patch is selected by NSO build
ID, not a firmware version string. The SD path affects every matching
Atmosphere boot using that card, not exclusively emuMMC. Other patch sets
can conflict; the installer does not audit those directories.

IPS SHA-256:
`1fffbb8e068c41c1e929748785fea007e04f7265b20c4086739fdd8d45802116`

## Evidence and reproducibility

Owner-local executable analysis found the `production_mode` key selector at
mapped address `0x9f460`. Retail selects modulus `0x220b24`; development
selects `0x220c25`. The size routine returns `0x101`; the public exponent
is 65537. Verification callers at `0x93f4c` and `0x94360` select this key.
The patch uses NSO offset `0x220c24` (mapped address plus the 0x100 header),
following [Atmosphere's loader patch handling](https://github.com/Atmosphere-NX/Atmosphere/blob/master/stratosphere/loader/source/ldr_patcher.cpp).

`cmd/newspatch` rejects a wrong build, modified source, unexpected original
modulus or non-RSA-2048/65537 key, and refuses an existing output path.
It expects the owner's uncompressed NSO, whose SHA-256 is
`dfe4c5361e91db1c508dedc6289fdd04d86f7990f4be6357f245649090733b5a`.

From the news module:

```sh
go run ./cmd/newspatch -nso /private/bcat-uncompressed.nso \
  -public-key tools/news-patch/openpak-bcat-public.pem -out /tmp/openpak-bcat.ips
go test -race ./...
go vet ./...
```

Host tests apply the IPS, check its exact range, verify OpenPak signatures
and reject modified signatures and unsupported inputs. Independent application
to the actual owner-local NSO confirmed only the modulus range changes and
that its replacement matches the signing key. These host checks alone do not
prove native BCAT acceptance; the later v0.19 hardware result does demonstrate
catalog container acceptance. Installation service access also succeeded.
The NRO compiles with warnings treated as errors.

Firmware and extraction keys remain outside the repository. The matching
private signing key is owner-local at
`~/openpak-news-private/openpak-bcat-signing.pem` (mode 0600).
Back it up privately: replacing it requires a new public key and patch.
Never distribute it in an NRO, SD archive or source repository.
