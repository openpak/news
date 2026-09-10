# Probe 0.19.0 build record

Adds separately managed News edge-token omission IPS on D-pad Right/Left;
R/L retain the public-key patch controls. Both installers check the running
BCAT build and preserve differing existing files. Reboot is required.
Hardware installation succeeded; after reboot ZL returned success with 155
decoded catalog bytes. The independent SSL comparison remains on ZR.

Built with -Wall -Wextra -Werror. NRO SHA-256:
`bed6903365f588371c91de514eb195893385e43b4a730a62730a00dcedb2201b`.

From the Openpak workspace root:

```sh
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/news/tools/news-probe:/work" -w /work \
  devkitpro/devkita64@sha256:1fc388c3a0d34bd2045a6dadcb1020e069d5f876a187fd705de14b4440c00282 make
```

## Previous build evidence

# Probe 0.18.0 build record

ZR reproduces the native catalog SSL context version flags 0x05000001 on
both ssl and ssl:s. Each pass uses a separate initialized/closed service;
system pass uses CreateContextForSystem and CreateConnectionForSystem.
Connections carry only the catalog GET, never a device client certificate.
The public OpenPak CA is imported per context as before. DNS is checked;
only the known owner server IP is contacted. ZL and the signing patch are
unchanged. No new code patch is justified by the device-PKI result alone.

Build passes -Wall -Wextra -Werror with the pinned toolchain. Hardware results
pending. NRO SHA-256: `1ed7c577391a91e8e71b6a93f89c96d183bbff7c52d547642cba1e7a0c172515`

## Previous build evidence

# Probe 0.17.0 build record

ZR now tests built-in device-client PKI registration in a separate local SSL
context. That context is never attached to a socket and is closed immediately;
no certificate/key bytes are read, exported or sent. The existing independent
DNS/TCP/TLS/HTTP transport probe then runs as before. This tests a possible
setup difference, not a confirmed native News failure cause.

Built with the pinned toolchain and -Wall -Wextra -Werror.
NRO SHA-256: `93873368e4f35b23d9823601285c1e18de4cf40e40bd8b1e921618c2f7b4397d`

## Previous build evidence

# Probe 0.16.0 build record

Adds ZR transport diagnostics after two native attempts returned SSL error
0x167B with no observed server handshake. Reports IPv4 DNS resolution and
whether it matches the owner remote address; connects only to that address.
Checks TCP with a 10-second deadline and TLS using nonblocking IPC with a
15-second handshake deadline. HTTP write/read each have a 10-second deadline.
The OpenPak public CA is imported into this temporary SSL context only.
No client credentials, response content or certificate bytes are logged.
Sockets and SSL services are closed after each diagnostic. DNS lookup itself
uses the system resolver's timeout. Native News behavior remains under ZL.

Pinned toolchain build passes with -Wall -Wextra -Werror. Hardware transport
service access remains unverified; this is a diagnostic, not a confirmed fix.
NRO SHA-256: `8502e482720d3a0eb70fb5a1cf921f91ae9eb97d6547aba73974da8669e43890`

## Previous build evidence

# Probe 0.15.0 build record

Adds R/L installation/removal of the pinned BCAT public-key IPS. Installation
checks Atmosphere and the running BCAT NSO build ID through pm:dmnt/ldr:dmnt.
Writes a temporary file, flushes and closes it, then renames it into place.
Existing differing patch files are preserved; removal checks exact contents.
The existing probe/article behavior is unchanged.

Built with the pinned toolchain below and `-Wall -Wextra -Werror`.
NRO SHA-256: `f880b5429da3fb8016633973810d4f9a5b467f14b528b5b02e291c3b333febbe`

Host validation: `go test -race ./...` and `go vet ./...` pass. The patch
applied to the owner's uncompressed NSO changes only the 257-byte modulus
region; a signature made by the private OpenPak key verifies against the
resulting public key. This does not validate the complete BCAT container
format or prove that Atmosphere has applied the patch on hardware.

## Previous build evidence

# Probe 0.14.0 build record

Adds read-only filesystem enumeration and metadata-candidate exports from
News system save 0x8000000000000090. The pinned toolchain below compiles it
with -Wall -Wextra -Werror. No fixture bytes/IDs or server routes changed.
Only read-mode filesystem calls are used against the News save; SD exports
preserve existing files. Hardware save access and cache availability await
this test. No tests were added for this bounded diagnostic-only change.

NRO SHA-256: `ea168abf44da3277d96da2a979ea2b48129116f29c572c5994e0f61295415f96`

## Previous build evidence

# Probe 0.13.0 build record

The 0.13.0 NRO adds command-30500 catalog downloading with a completion
event, 30-second wait, cancellation and bounded memory-permission recovery.
It exports only a nonzero bounded decoded result, preserves existing SD
exports, and leaves retained transfer memory untouched if the service has
not released it. No new test article bytes or IDs were generated.

NRO SHA-256:
`52e48403a43adff1ee618f7c892b13fa85d8fd270337c30e1c96361c63319c98`

Built using the same pinned toolchain and build command recorded below,
with warnings treated as errors. Go vet and full race tests pass. The new
catalog fixture generator was independently decoded with Python MessagePack;
channel field types, matching detail and original JPEG dimensions pass.
Hardware 0.13.0 now confirms downloader creation and event completion.
GetResult returns News 125-81 and decoded size is zero with the current
JSON response. The exact meaning of description 81 is not established.

## Previous 0.12.0 fixture/build evidence

### Probe 0.12.0 build record

Built 2026-09-08 on linux/amd64 with:

```text
devkitpro/devkita64@sha256:1fc388c3a0d34bd2045a6dadcb1020e069d5f876a187fd705de14b4440c00282
libnx 4.12.0-1
devkitA64 r29.2-1
switch-tools 1.13.1-1
host cjpeg: libjpeg-turbo 3.2.0 (build 20260701)
```

Generate headers from the `news` repository root:

```sh
go run ./cmd/newsfixture -published 2026-09-08T21:53:14Z -out tools/news-probe/source/test_article.h
go run ./cmd/newsfixture -published 2026-09-08T21:53:14Z -cleanup -out tools/news-probe/source/cleanup_articles.h
```

Build from `tools/news-probe`:

```sh
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD:/work" -w /work \
  devkitpro/devkita64@sha256:1fc388c3a0d34bd2045a6dadcb1020e069d5f876a187fd705de14b4440c00282 make
```

SHA-256:

- Live fixture (25142 bytes): `2ab40068e2de42852d22d87f1fa9f2ca8e6a76d8c05e328ecabaa32604f87ab9`
- Cleanup 1909260910 (25137 bytes): `c25cd55487ad12cba55cbb6285eeef1af5b1a583280d00d65ee07519d70f6747`
- NRO: `1681dfa61dd7b3b126e643ee2606e49138f7d5203ab9f87828b8512dde1d285f`

The generator emits only the current test ID and its cleanup.
The live fixture expires 2026-09-15T21:53:14Z. The probe refuses live
posting outside its time window and refuses unknown stored content.

Validation: Go fixture tests pass, including ordered map encoding. An
independent Python MessagePack decoder/re-encoder reproduces each payload
exactly and verifies local schema, ID, channel, expiry and JPEG dimensions.
The unchanged C payload-guard logic passed host ASan/UBSan in 0.11.0: live and matching
cleanup bytes accepted, wrong-ID, changed and truncated bytes rejected.
The NRO compiles with `-Wall -Wextra -Werror`.

Hardware posting and rendering are now confirmed by stored-byte readback and
the owner's successful News test. Expiry replacement is verified, but the
immediate post-cleanup check still found the record; removal is unconfirmed.
The 0.10.0 private snapshot confirms the prior two tests are absent.

Independent decoding verifies both payloads differ from the recorded 0.11.0
payloads ONLY in news_id (1909260909 to 1909260910), by restoring the old ID
and reproducing the prior SHA-256 hashes. Timestamps and all formatting
remain unchanged. No additional test IDs are allocated at runtime.
