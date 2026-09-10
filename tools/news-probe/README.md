# OpenPak News Probe

Hardware experiment for [native News](../../docs/native-news-reopen.md).
Version **0.19.0** adds a separate News edge-token omission patch.

## Current v0.19 test: skip the pre-download edge-token request

Copy `openpak-news-probe.nro` to `sd:/switch/openpak-news-probe/`, replacing
the previous NRO. With OpenPak DNS enabled, launch it and press **D-pad Right**
to install the new patch. Wait for the installation result, exit with **+**,
and reboot Atmosphere. Relaunch and press **ZL**, then exit with **+** and
copy back `report.txt` (and `catalog-v013.msgpack` if a successful decode
exports it). The export filename is retained from the original diagnostic.

Keep the existing BCAT verification-key patch installed; R/L still manage
that separate patch. **D-pad Left** removes only the new token patch, followed
by a reboot. It affects the pinned build's News memory downloads for every
URL while installed, so keep the console routed to OpenPak. It does not patch
the global dauth service or the shared game BCAT token helper. Native success
is not yet confirmed; this test isolates the pre-download token dependency.
See [patch details and rollback](../news-patch/news-no-dauth/README.md).

Hardware result after reboot: **GetResult 0, 155 decoded bytes**, with a matching
server catalog request. Next compare the exported `catalog-v013.msgpack` with
the original fixture and test **News → Find Channels**. The probe's successful
download alone does not create a subscription or post articles.

## Previous v0.18 test: native SSL profile

Use the rebuilt **v0.18** NRO and press **ZR**. This compares the native
catalog downloader's SSL API 5/Auto-TLS profile on the standard and system
SSL services. Wait until finished, exit with **+**, then copy back the report.
No reboot or patch installation is required. Both connections use the owner
server IP and send only a catalog request, without device credentials.

Hardware result: both profiles succeed, with matching server catalog requests.
The subsequent ZL run returns **0x167B (123-11)** with zero decoded bytes
and no new TLS hello or request logged by the fixture server. Native retrieval
is still unresolved. No repeat of the unchanged test is needed; direct HTTP
success does not verify native download or container decoding.

## Previous diagnostics

## Current test: check device-certificate setup

Replace the probe NRO with v0.17, launch and press **ZR**. Wait for the
transport diagnostic to finish, exit with **+**, then copy back the report.
The new `PKI register device cert` result checks a separate temporary SSL
context that is never connected to the network. No certificate/key bytes
are exported or sent. Existing DNS/TCP/TLS checks follow as before.
No reboot or patch reinstallation is needed for this diagnostic.

## Previous v0.16 transport test

## Current test: locate the SSL connection failure

Replace `sd:/switch/openpak-news-probe/openpak-news-probe.nro` with v0.16.
Launch and press **ZR** (right trigger). Wait for `Transport diagnostic
finished`, exit with **+**, then copy the updated report back privately.
This checks DNS, TCP, TLS and an HTTP request to the owner's remote server.
If DNS points elsewhere, it reports that and tests the explicit OpenPak IP
without changing DNS or contacting the unintended destination. The public
OpenPak CA is trusted only in this temporary test context. Success here does
not prove that BCAT's own SSL context has the same behavior or trust setup.
No patch installation/removal, article posting or subscription operation is
performed. ZL retains the native News download test; R/L remain patch controls.

## Previous v0.15 patch installation

## Current test: install the BCAT key patch

Copy `openpak-news-probe.nro` to `sd:/switch/openpak-news-probe/`.
Launch v0.15.0 in Atmosphere and press **R**. Installation checks the running
BCAT module build ID and refuses unknown builds. After successful installation,
exit with **+** and reboot Atmosphere. **L** removes this exact patch; reboot
again to restore the original key. Removal also works after a firmware update.
A differing existing file is preserved. Reports append to
`/switch/openpak-news-probe/report.txt` as before.

The patch changes the retail BCAT verification key for News **and game BCAT**.
It applies to matching Atmosphere boots sharing this SD, including sysMMC if
booted through Atmosphere. It is not restricted to emuMMC by the file path.
It does not post articles. Signed/encrypted server containers are still needed;
Find Channels is not expected to start working solely from this installation.
Hardware installation and native container acceptance remain unverified.
See [patch evidence and manual rollback](../news-patch/README.md).

The first [signed-container server experiment](../../docs/native-news-container.md)
is now deployed. Use **ZL** in this same v0.15 build to test native decoding;
expected successful decoded catalog size is 155 bytes. Copy back the report
and `catalog-v013.msgpack` if created. The candidate AES profile and actual
patch application still need that hardware result. Do not press B for this test.

## Previous v0.14 cache inspection

## Current test: inspect cached channel metadata

1. Replace `sd:/switch/openpak-news-probe/openpak-news-probe.nro`.
2. Launch **0.14.0** and press **X**.
3. Wait for `Cache inspection ended` or a save-open error, then press **+**.
4. Copy `report.txt` and any `cache-v014-*.bin` files from the probe folder
   to your computer for private inspection.

X attempts to open only the documented News system save, first in System
space, then User space if needed. It lists paths and sizes and exports files
whose paths suggest catalog/topic/detail metadata. It uses only filesystem
read operations on that save; no commit, write, rename or delete operations.
It excludes candidate paths containing key, secret, token or passphrase.
Bounds: depth 4, 512 entries, 64 export attempts, 4 MiB per file and 16 MiB
read for exports. SD outputs are exclusive-create and existing files remain.
Names are only candidate heuristics: an export is not a verified catalog,
and failure to find a named candidate is not proof no catalog exists.
The save may be locked or inaccessible while News owns it; report the error
rather than changing permissions or stopping system services.

A/B/Y retain the old targeted cleanup/post/inspection controls; do not use
B for this cache test. ZL runs the previous native downloader diagnostic.

## Previous v0.13 downloader experiment

Version **0.13.0** adds a native catalog-download diagnostic.

## Run catalog diagnostic

1. Replace `sd:/switch/openpak-news-probe/openpak-news-probe.nro`.
2. Launch from hbmenu and confirm **0.13.0**.
3. Press **ZL** in v0.14 (**X** in v0.13). Allow up to 35 seconds for download and memory cleanup.
4. Press **+**, then copy back `report.txt` and `catalog-v013.msgpack`
   if that file was created, from `/switch/openpak-news-probe/`.

X submits the known catalog URL to the News service's command 30500, using
4 MiB transfer memory. It logs creation, completion, result and decoded
size. A successful nonzero result is exported privately for host validation.
Only one attempt is allowed per launch. X does not post articles, set
passphrases or change subscriptions. The service may perform its normal
network/cache work. Current DNS routes determine which server receives it.
The IPC wrapper follows unofficial Switchbrew documentation and has not
been exercised on the owner's firmware yet. A wrapper failure is not proof
of catalog rejection or signature enforcement.

A/B/Y retain the 0.12.0 cleanup/post/readback actions described below; **use
X for this experiment**. Their existing test payloads and v012 export names
are unchanged. Native rendering was confirmed in 0.12.0; the catalog path
remains separate.

## Previous local-format test (0.12.0 payload)

Version **0.12.0** tests the built-in local article layout: version 1/1,
NORMAL display, and a single body map. The owner-selected channel remains
`nx_news`; prose and graphics are original. It uses one fresh test ID, 1909260910, to test whether the previously
expired ID was suppressing posts. Subsequent presses reuse this same ID.
The payload differs from 0.11.0 only in that numeric ID.

The owner confirmed native rendering on HOS 22.5.0 with this build.
Readback matches the live fixture exactly. Cleanup writes the expired
payload successfully, but immediate record removal is not yet confirmed.

### Local-format test controls

1. Replace `sd:/switch/openpak-news-probe/openpak-news-probe.nro`.
2. Launch from hbmenu and confirm **0.12.0**.
3. Press **B**. Wait for post, exact-payload readback and export results.
4. Press **+**, open News, and check **OpenPak local-format test** for its
   card image, body text and blue/green image. IPC success alone is insufficient.
5. After checking, relaunch this same build and press **A** to expire only
   its verified test record. The report says whether the target disappeared.
6. Copy back from `/switch/openpak-news-probe/`:
   - `report.txt`
   - `news-database-v012.sqlite` (snapshot from the B action, before cleanup)
   - any `stored-v012-*.msgpack` files that were created.

Repeated B presses do not repost an identical stored fixture. A does nothing
if the target is absent. Both actions enumerate the bounded inventory and
refuse to overwrite a target with unknown bytes or a populated user ID.
Cleanup submits an expired replacement for this ID; it is not a documented
single-article deletion API. No ClearStorage or direct database writes occur.
Keep this build until its test is cleaned up: regenerated fixtures have new
bytes and are deliberately not authorized to overwrite arbitrary old content.

Y retains read-only inspection, including the built-in local reference.
It reads at most 512 records, 32 MiB total and 4 MiB per article. Existing
exports are preserved. X now runs the catalog diagnostic described above.
Private archives and database snapshots belong outside repositories.

## Build

Install `cjpeg` from libjpeg-turbo on the build host (used for baseline
JFIF with 4:4:4 sampling). Generate the embedded fixture from the `news`
repository root first:

```sh
go run ./cmd/newsfixture -out tools/news-probe/source/test_article.h
go run ./cmd/newsfixture -cleanup -out tools/news-probe/source/cleanup_articles.h
```

For reproducibility, use `-published 2026-09-08T21:53:14Z` for both
commands, as recorded in `BUILD.md`. Cleanup payloads expire one hour
before that time; the live fixture expires seven days afterward.
Then use devkitPro devkitA64 with libnx. From this directory:

```sh
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD:/work" -w /work devkitpro/devkita64:latest make
```

For reproduction use the image digest and package versions recorded in
`BUILD.md` instead of `latest`. Compilation enables `-Wall -Wextra
-Werror`. Console service behavior requires hardware; host compilation
cannot verify the IPC interfaces on the owner's firmware.

This small C/libnx hardware diagnostic is the native tooling exception
for the owner's 2026-09-07 request to test the proposed homebrew route.
It is not a custom sysmodule; Go remains the server implementation.
Source license: AGPL-3.0-only, covered by the repository's `LICENSE`.
The build links libnx; see its [upstream license](https://github.com/switchbrew/libnx/blob/master/LICENSE).
