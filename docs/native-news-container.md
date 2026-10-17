# Native News signed-container experiment, 2026-09-09

**Current deployment (2026-09-10):** owner-authorized v006/cdn-v003 now serves four channels, populated archives, twelve summaries and linked full articles. Host and public HTTPS verification pass; native rendering and automatic subscription delivery remain pending. See [current delivery, evidence and rollback](native-news-article-delivery.md). Earlier empty-archive/preparation statements below are historical.

## Channel lineup seed prepared; multi-channel fixture CDN built (2026-09-10, local only)

The OpenPak channel lineup is now authored as source files:
[`channels/`](../channels/README.md) defines four channels —
`openpak_news`, `openpak_games`, `openpak_workshop`, `openpak_help` — with
three opening articles each, written from verified evidence and illustrated
with original generated artwork. `cmd/newsseed` builds decoded payloads
from those sources (deterministic under a pinned `--published`; a rerun
updates in place), and `tools/news-cdn/sign-seed.sh` signs them with the
owner-private keys and verifies every container against
`tools/news-patch/openpak-bcat-public.pem` using openssl.

Host validation of the prepared seed, all passing:

- Independent MessagePack decode of every payload: catalog is an array of
  four records; per-channel detail carries an empty `latest_news_urls`;
  archives are exactly `{na_required: false, data_list: []}` — the same
  decoded shape as the deployed v004 archive fixture (313-byte containers).
- All 12 articles follow the locally proven v0.12 NORMAL layout with the
  exact field order; image dimensions verified (icon 70×70, card 358×201,
  hero 800×364, baseline JFIF 4:4:4, 3 components); stable numeric news
  ids below 2^32; expired variants keep identity with a past window.
- 37 signed containers (catalog, 3 per channel, live+expired per article)
  verified against the public OpenPak key.
- `cmd/newscdn` serving the signed tree answers all 13 routes with
  `application/octet-stream`, `Cache-Control: no-store`, byte-exact
  fixtures; per-topic routes are derived from the fixture tree and unknown
  topics fall through to the adapter fallback as before.

Not deployed and not claimed as delivered: the remote experiment is still
image v004 with the old single-channel fixtures. The new catalog decodes to
720 bytes host-side — only the one-channel 155-byte catalog is
hardware-proven. The archive `data_list` entry format remains untraced, so
no article is served anywhere and the signed article containers are
prepared inputs for the delivery-path work, nothing more. Deployment steps,
update/retire behaviour and the ordered hardware checks are in
[`channels/README.md`](../channels/README.md). Rollback of this local
work is simply not deploying it; the deployed v004 service is untouched.

## Channel archives response added in server v004

The owner observed “failed to load channel information” while logs repeatedly
showed the unimplemented OpenPak online_archives route. Owner-local qlaunch
analysis locates the route formatter at `0x653ea8`, native download at
`0x653fdc`, and decoded response parsing at `0x6540a8`. The top-level map
recognizes `na_required` (boolean) and `data_list` (array); the empty array
terminator is handled at `0x6549b0`, and the final map terminator marks success
at `0x654bb0`. It then requests the channel icon. This supports an empty
archive fixture without inventing article entries.

Server v004 now handles `/api/nx/v2/topics/openpak_news/online_archives` with
a signed/encrypted container for `{na_required: false, data_list: []}`.
Decoded size is 25 bytes; container size is 313 bytes; SHA-256 is
`1026b7fecd36330da8fbc72f1d0c300832049d9ed90333469cdda7c6278d4ee2`.
Original catalog/detail/icon files are unchanged. No article is posted.
Route tests and vet pass. Independent host AES decryption, RSA verification
and MessagePack parsing pass. Deployed isolated image v004 has digest
`31e5f0697173fd3a836a31b6aba10c10c580c31f8e23baef2411f35c8b90986b`.
Public HTTPS checks return exact archive and catalog fixture bytes with CA
verification. Channel-page hardware acceptance remains pending; retry the
OpenPak channel without changing the console patch or NRO.

Rollback this server change by selecting the previous v003 image in the
experimental quadlet, daemon-reloading and restarting that service. The extra
archive fixture can remain on disk; v003 does not load it.

## News UI follow-up reaches catalog, icon and detail

Server logs at 18:51:04–13 UTC show catalog, OpenPak icon and OpenPak detail
fixture requests following the native probe success. The console then repeatedly
requests `/api/nx/v2/topics/openpak_news/online_archives`, which the fixture
server does not implement and forwards to the adapter fallback. The actual
fallback status/body was not captured here. This identifies a missing signed
route to investigate before claiming a working channel page or delivery.
Requests for existing Nintendo channel details also reach the fallback.
The logs establish that the console uses the catalog to request OpenPak assets;
visual rendering and subscription state still require the user's observation.

## Native catalog download and decoding succeeded

After reboot, probe v0.19 at epoch 1788979797 returns GetResult **0** and
decoded size **155 bytes**, exporting `catalog-v013.msgpack`. The server
confirms a native catalog request at **18:49:45 UTC**, serving the 443-byte
signed/encrypted fixture. This is the first successful native retrieval and
container decode with the OpenPak key and News token-omission patches.
It supports the pre-download token dependency as the previous blocker and
hardware acceptance of this fixture's cryptographic profile. The exported
plaintext still needs byte-for-byte comparison; matching length alone is not
that comparison. Find Channels parsing, detail/icon rendering and subscriptions
remain untested. No articles or subscriptions are changed by this ZL probe.
The article count remains 13 after the earlier accidental B press.

## Pre-download edge-token dependency found; v0.19 prepared

Tracing backwards from the memory downloader found a separate authentication
stage before curl: worker `0xfc5d0` → `0xfc760` → `0xfecc0` → `0x90f70` →
`0xb59f0` / `0xb5af0` → `dauth:0` command 10 (EnsureEdgeTokenCacheAsync),
then command 11 (LoadEdgeTokenCache). The later request uses the resulting
`X-Nintendo-DenebEdgeToken` header. Errors from this prerequisite can reach
the download result before any catalog TLS connection. This is a stronger
lead than further changes to the catalog connection's SSL profile; the exact
failing operation inside dauth is still unconfirmed.

Prepared [a separate token-omission IPS](../tools/news-patch/news-no-dauth/README.md)
and probe v0.19 to install/remove it with D-pad Right/Left. It changes only
the News memory worker's call site and explicitly empties its token buffer.
Shared token functions, global authentication services and signature checking
instructions are untouched. It is not hostname-selective: keep OpenPak DNS.
Generator tests, actual-NSO patch scope checks, ARM64 worker emulation and the
NRO build pass. Hardware test: install with D-pad Right, reboot, then ZL.
Keep the earlier BCAT verification-key patch installed.

The first v0.19 report (epoch 1788979605) confirms the running-build check and
successful token-patch installation. It contains no subsequent reboot/run or
ZL result, so runtime behavior is still pending. An accidental B press found
the known test ID as the exact expired fixture and attempted to repost that
same ID. PostLocalNews returned success, but the verification scan then found
the target absent and the record count fell from 14 to 13. No new stored test
article was found. Existing export files were preserved (errno 17). No extra
cleanup is needed for that target based on this scan.

## Device PKI result and native-profile comparison

Probe v0.18 at epoch 1788978655 succeeds with **both** ssl and ssl:s using
API 5/Auto TLS: context creation, connection creation, handshake and an HTTP
first read of 512 bytes all succeed. DNS matches the OpenPak server. Server
logs confirm two catalog fixture responses at 18:30:46–47 UTC. These direct
tests do not reproduce the native failure by matching its SSL version and
service alone. Both contexts imported the public OpenPak CA and reported a
verification mask of zero; they do not establish native certificate setup
or successful container decryption.

The subsequent v0.18 run at epoch 1788979021 tests ZL: creation of the
native download and completion event succeeds, but GetResult returns
**0x167B (123-11)** and decoded size is **zero**. The fixture server log
still ends with the earlier two direct requests; no new TLS hello or catalog
request is recorded. Native retrieval remains unresolved before any observed
fixture response, so this run provides no catalog-schema, AES-key or RSA-patch
validation. Repeating the unchanged ZR/ZL tests adds no useful distinction.
Next investigation is native SSL initialization and curl backend setup beyond
the already tested context version and service selection. The device PKI
failure is a candidate lead, not a confirmed cause on this path.

Probe v0.17 at epoch 1788977652 reproduces 0x167B in local-only
`sslContextRegisterInternalPki(DeviceClientCertDefault)`. Its independent
TCP/TLS/HTTP path succeeds. This establishes that device PKI registration
fails on this console, but does not prove that the catalog path calls it.

Further owner-local disassembly distinguishes two callbacks:

- `0xf0e20` calls the SDK RegisterInternalPki wrapper at `0x1f07f0`.
  It is installed via the pointer at `0x277ed8` by code at `0xc533c`.
- The native memory downloader instead installs callback `0xfd6c0` through
  pointer `0x278218` at `0xfd59c`. This callback calls context creation
  `0x1f0580` with flags **0x05000001**; it does not itself call the PKI wrapper.

[SSL version documentation](https://switchbrew.org/wiki/SSL_services) identifies
bits 24–31 as the API version. These flags select API 5 and Auto TLS, unlike
the original direct probe's explicit TLS 1.2/API 0. The shared SDK context
wrapper also supports ssl:s CreateContextForSystem. Probe v0.18 compares
API 5/Auto on ssl and ssl:s, including the system connection API. It never
registers or transmits a client certificate in these connected contexts.
The local-only PKI failure remains a separate finding. The BCAT key IPS is
unchanged; no speculative certificate-bypass patch has been issued.

## DNS corrected; native SSL setup still fails

After the corrected installer and reboot, the v0.16 report at epoch
1788977470 shows DNS matching 145.241.199.19 and successful direct TCP/TLS/HTTP.
The corresponding server request is logged at 18:10:57 UTC. The native ZL
request still returns 0x167B with zero decoded bytes and no second handshake.
Thus correcting the route was necessary but did not resolve native SSL setup;
the earlier routing explanation did not account for every failure.

Probe v0.17 adds a local-only `sslContextRegisterInternalPki` diagnostic for
the default device client certificate, in a separate context never attached
to a socket. This checks one possible distinction between the direct probe
and native services; it is not evidence yet that News fails at this exact call.
No credential bytes are exported or transmitted by that context. The original
transport probe follows. Results are recorded in the section above.

## Routing cause found in installer

Probe v0.16 twice reports `DNS matches OpenPak remote: no`; connecting to the
explicit owner server IP succeeds at TCP, SSL handshake and HTTP. Server logs
confirm those diagnostic requests at 18:04:31 and 18:07:10 UTC. The SSL verify
mask is 0 on this patched console, so this is transport reachability evidence,
not proof of certificate verification or of the temporary CA import's necessity.
The native ZL request still returns 0x167B and no decoded bytes.

The owner's OpenPak setup tool `console/openpak-nro/source/hosts.c` omitted
`bcat-topics-lp1.cdn.nintendo.net`. Added that exact served hostname to its
managed redirects. The existing enable/disable/reapply tests pass. Re-selecting
OpenPak with the rebuilt installer rewrites its managed block in both default
and emuMMC hosts files; reboot is required. It uses the installer-configured
server address, expected to be 145.241.199.19 for this experiment. This does
not install or remove the separate News signing-key patch.

Retest ZR after reboot: DNS should match. Then ZL can finally test the signed
container through the native service. Prior TLS compatibility changes are not
established as necessary; the observed routing mismatch explains why those
attempts never reached this service.

## Second hardware result: transport diagnostic required

The next report (probe epoch 1788974810, 17:26:50 UTC) still returns
`0x167B` and zero decoded bytes. The v003 log has no matching TLS hello or
HTTP request; the earlier 17:24:43/44 entries are the host validation request.
The TLS compatibility changes therefore have not resolved the hardware failure.
Probe v0.16 adds a direct DNS/TCP/SSL diagnostic on ZR, with a temporary trust
context containing only OpenPak's public CA in addition to system defaults.
It tests the owner IP only and does not alter DNS or the native News settings.
This distinguishes route/connectivity and SSL service setup from the BCAT
container test. Neither the running key patch nor AES profile is yet validated.

## First hardware result and TLS follow-up

The first v0.15 download after reboot returned `0x167B` (123-11), zero decoded
bytes, with no corresponding HTTP request in the fixture service log. The
[Switchbrew error table](https://switchbrew.org/wiki/Error_codes) identifies
this as the SSL module's `ResultErrorLower`. It does not establish the exact
TLS failure or test the BCAT signature/AES profile.

Service image v003 aligns the console-facing listener with the working adapter:
legacy TLS versions/suites, explicit non-hybrid curves, no client certificate
request, and a complete leaf-plus-CA certificate chain with server-auth and
key-usage extensions. TLS hello diagnostics record only protocol versions and
cipher IDs. HTTP/1.1 is used explicitly because HTTP/2 rejects legacy cipher
suites. An external TLS 1.2 RSA/AES-CBC request returned the exact 443-byte
catalog after this adjustment. Fixture bytes are unchanged. The fallback connection continues to
verify its upstream certificate. Host tests and vet pass; a new hardware attempt
is needed to determine whether this resolves the SSL failure.

The report also records L removal followed by R installation. The final SD
state is installed; these actions do not themselves change the running module.
The owner already reported rebooting after the initial installation. Relaunch
the probe for one fresh ZL attempt; L/R do not need to be pressed again.

The owner confirmed probe v0.15 installed the exact-build BCAT key patch and
reported rebooting Atmosphere. Installation is evidenced by successful
pm:dmnt/ldr:dmnt calls and the installer success message in the owner's report.
Execution of the patched verifier is still awaiting a successful native download.

The owner's subsequent instruction to implement the server response authorizes
this isolated hardware experiment. The remote News topics SNI now points to
`openpak-news-experiment`, which serves three original pre-signed fixtures.
Other paths on that host are forwarded to the existing adapter over HTTPS,
with its CA and server name verified. Other SNI routes and nx-baas source are
unchanged. This is a scoped exception to the PRD's previous hostname ownership
boundary; it is not the production adapter integration or completed NB-1.

## Container evidence

[Switchbrew's container description](https://switchbrew.org/wiki/BCAT_Content_Container)
documents the 0x120-byte header, AES-CTR, RSA modes and PBKDF2 derivation.
Owner-local HOS 22.5.0 BCAT analysis additionally confirms:

- Native News memory downloader `0xfdf00` decrypts with `0x92cd0`/`0x92d50`,
  then hashes the decoded bytes at `0xfe0bc` and verifies the header's
  signature at `0xfe0d0`. The header itself is not included in that hash.
- This uses `0x93f30`, the same retail-key selector covered by our IPS.
- Per-title downloader `0x91840` likewise writes decrypted data and verifies
  that decoded file through `0x94330`/`0x943c0`.
- AES derivation at `0x93820` uses PBKDF2-HMAC-SHA256, 4096 iterations,
  lowercase 16-character title ID followed by the selected salt string.
  Retail index 0 resolves through table `0x220870` to a 16-character string
  at `0x2208f0`. No salt values are included in the repository.
- News obtains its title ID and passphrase from `news-sys:/passphrase.bin`
  (`0xfe4d0`), parsing a title ID followed by a colon and passphrase.

The first fixture uses the historical qlaunch title ID/passphrase profile
documented by the original [BCAT-Manager decoder](https://github.com/Random06457/BCAT-Manager/blob/master/Bcat%20Manager/Bcat%20Manager/BCAT.cs),
with the salt extracted from owner-provided firmware. **That passphrase was
not found verbatim in the checked firmware modules and is not verified against
the console's saved configuration.** The derived AES key is a candidate pending
hardware testing, not a confirmed current News key. Nothing changes the
console's passphrase. The private inputs remain outside the repository.

`internal/bcatcontainer` emits version 1, crypto type 3 (AES-256-CTR for this
fixture), RSA/hash type 1 (PKCS#1 v1.5/SHA-256), index 0, a fresh random IV,
and an OpenPak signature over plaintext. It also supports AES-128/192 using
caller-provided derived keys. It does not embed platform secrets or implement
a Nintendo signing capability.

## Deployed fixture v001

| Path under `/api/nx/v1/topics/` | Container bytes | Decoded bytes |
| --- | ---: | ---: |
| `catalog` | 443 | 155 |
| `openpak_news/detail` | 460 | 172 |
| `openpak_news/icon` | 2025 | 1737 |

Catalog SHA-256:
`e21ab4a499597e1d6142ca247ae0be9fcfd9e898fc0fcb254101f48ff8ed4001`

The decoded catalog is an array with one original OpenPak News channel.
Detail has an empty `latest_news_urls` array. Icon is an original JPEG.
No new article is posted and no subscription operation is performed by the
probe. Channel schema compatibility remains a separate test after decoding.
The proven local article schema and fixture ID are unchanged.

The encoder is `cmd/newscontainer`; the standalone fixture service is
`cmd/newscdn`. The CDN receives only prepared containers, its TLS leaf key,
and public CA certificate. Content-signing and AES keys stay on the owner's
machine. Known responses are `application/octet-stream`, with explicit length
and `Cache-Control: no-store`. Query values and credential headers are not logged.

## Validation and hardware step

`go test -race ./...` and `go vet ./...` pass. Tests check framing, all three
AES key sizes, partial CTR blocks, plaintext signatures, corruption rejection,
fresh IVs, invalid inputs, exact routes and adapter fallback selection.
An independent Python cryptography implementation decrypted all three actual
fixtures and verified their signatures using the public key embedded in the IPS.
MessagePack decoding confirmed an array catalog and map detail.

The remote ARM64 service is active; its TLS leaf verifies against the existing
OpenPak CA. The public SNI endpoint returns the exact generated catalog bytes.
These host checks do not establish the console's current passphrase or prove
that the loader applied the patch.

Use the existing **v0.15 NRO**, press **ZL**, wait for completion, exit with **+**.
Copy `/switch/openpak-news-probe/report.txt` back privately. If created, also
copy `catalog-v013.msgpack`. A successful download must have result 0 and
decoded size 155, and exported bytes must match the original catalog fixture.
The export filename remains v013 because that diagnostic implementation has
not changed; existing exports are preserved by exclusive creation.

On failure, compare the new native result and server request log. Do not infer
a symbolic error name from the numeric result. In particular, success is needed
before claiming the key patch or candidate AES profile works on hardware.

## Remote layout and rollback

Remote host: owner-managed `<prod-host>`. Quadlet:
`~/.config/containers/systemd/openpak-news-experiment.container`.
Files: `~/openpak/deploy/news-experiment/`. Internal listener: 20040 on the
existing container network, with no new published port. Static image:
`localhost/openpak-news-experiment:v003`. It is an experimental snapshot,
not an automatic production deployment.

Traefik dynamic configuration adds only the `news-experiment` TCP router and
service for exact SNI `bcat-topics-lp1.cdn.nintendo.net`. Its backup is
`~/openpak/deploy/traefik/dynamic.before-news-v001.yml`. To roll back, remove
those two entries from the current dynamic configuration, verify that requests
reach the adapter again, then stop the experiment service. Do not restore the
whole backup over subsequent unrelated routing changes. Remove the quadlet
and reload user systemd if the experiment should also remain disabled at boot.

The experiment TLS certificate expires after 30 days and requires renewal if
retained. Console rollback remains **L** in probe v0.15 followed by an
Atmosphere reboot, as described in the patch README.

### TLS renewal — 2026-10-12

During the Splatoon 3 BCAT investigation, the retained topics-server leaf was found expired
(October 9), with nx-baas logging failed upstream verification for OpenPak News lists.
The leaf and chain were renewed using the existing OpenPak CA, CSR and server key; public
TLS hostname/chain verification now succeeds, and the adapter's News relay returns HTTP 200.
The renewed leaf expires October 12, 2027. Only the News experiment service was restarted;
content-signing keys, content and console patches were not changed. Old public certificates
are backed up in the remote TLS directory under `*.before-20261012-132534`.
This repairs a confirmed TLS defect; Splatoon 3 lobby success remains unconfirmed.
