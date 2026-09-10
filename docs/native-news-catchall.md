# Native News catch-all evidence — 2026-09-07

Read-only inspection of the running local test stack. Sources are
`/tmp/snirouter.log` and `/tmp/nx-baas.log`; raw logs remain outside the
repository. This record omits request credentials and device identifiers.

## Observed setup

The running SNI router listens on 21999 and the Switch adapter listens
on 21000. The router's local source sends non-device-auth hosts to the
adapter. The adapter's default host handler logs the request and returns
HTTP 404 with `{}`. No adapter source or live routing was changed.

The adapter log shows successful HTTP requests for these News surfaces:

| Host | Path | Result |
| --- | --- | --- |
| `bcat-topics-lp1.cdn.nintendo.net` | `/api/nx/v1/topics/catalog` | 404 |
| `bcat-topics-lp1.cdn.nintendo.net` | `/api/nx/v1/titles/<title-id>/topics` | 404 |
| `bcat-list-lp1.cdn.nintendo.net` | `/api/nx/v1/list/nx_news_<feed>` | 404 |
| `bcat-list-lp1.cdn.nintendo.net` | `/api/nx/v1/list/nx_data_<title-id>` | 404 |

Catalog requests at log timestamps 14:36:21, 14:40:10, 14:45:01 and
15:00:55 carry `l=en-GB` and two `c[]=GB` parameters. A per-title request
at 15:09:52 carries one `c[]=GB`. These are log timestamps, not an
independent measurement of when the owner opened an applet.

The client user agents identify `nnNews` or `nnBcat` with SDK `22.2.0.0`.
That is an SDK string, not an exact Atmosphere version or a replacement
for the probe's HOS version. The owner reports current firmware and CFW.

## Interpretation and next experiment

For these requests, the console gets through routing and TLS to the HTTP
handler. The current immediate failure is a missing handler. A 404 does
not test BCAT signature acceptance, payload decoding, channel registration
or article rendering. Serving invented JSON would not resolve the known
container-format requirements.

The owner opened News / Find Channels during this session and confirmed
completion. The live watch captured new catalog requests at 15:14:10 and
15:14:12, both `en-GB` with two `c[]=GB` values, both answered with 404.
This confirms fresh native News traffic reaches the catch-all. The
on-screen result has not yet been reported.

Independently run the diagnostic NRO to
check local News posting-service access. That local IPC path does not
use DNS or the catch-all and may allow a foreground helper to import
OpenPak content; actual posting and rendering remain untested.

## Remote Find Channels trace — 2026-09-08

The catch-all moved to the remote OpenPak host. Access uses the owner's
existing SSH agent socket <ssh-agent-socket>. The adapter runs as the
rootless Podman container openpak-nx-baas and user systemd unit
openpak-nx-baas.service. Use the unit journal for watches so container
replacement does not silently terminate capture. Raw credentials and
headers must not be copied into the repository or displayed in watch output.

At 22:08:17 and 22:08:18 UTC, the owner's Find Channels action produced
GET bcat-topics-lp1.cdn.nintendo.net/api/nx/v1/topics/catalog with l=en-GB
and two c[]=GB parameters. Each request appeared in multiple log entries;
these represent two request groups, not six separate calls. Journal access
summaries record HTTP 200 for both. The owner reported "failed to load
channel information". This differs from the earlier local 404 failure.

The container was replaced at 22:09:03 UTC during investigation. The active
image identifies version 0.7.22, revision
94530688ee7fcdb0ba75cc2ede27f06c0657c929. A subsequent unauthenticated
read-only GET pinned to the owner's server returned 37 bytes of JSON with
keys topics (empty array) and updatedAt (integer), not a BCAT container.
SHA-256: 46863c0843bf97afcd2b03c5d7a61e7f70aa3684b747e3aece71ce51770f7362.
The exact body of the earlier authenticated console response was not captured;
do not assume it was identical across the deployment change.

The current response contains no channels to subscribe to. HTTP success
does not demonstrate the correct catalog wire format. These observations
establish reachability and a still-failing discovery journey, not whether
the applet rejected JSON specifically, required metadata, or signature rules.
Next work is a verified catalog schema/container experiment and adapter
contract, followed by detail/icon and follow/unfollow observations. No
adapter code, routing or live responses were changed by this investigation.

## Public catalog decoder found — 2026-09-08

Online search located Random06457/BCAT-Manager, which implements a catalog
browser, beyond the endpoint-only NintendoClients/Pretendo documentation.
Its BCAT.cs GetRawNewsCatalog calls the topics/catalog endpoint through a
BCAT-decryption helper; GetNewsCatalog and NewsCache.RefreshCatalog decode
the resulting bytes as a MessagePack list of channel records. The modeled
fields are topic_id, name, publisher, description (strings), publishing_time
and last_posted_at (unsigned timestamps), and important (boolean).
Topic detail additionally models latest_news_urls. Icon downloads also use
the container-decryption helper before image decoding.

Sources:
- https://github.com/Random06457/BCAT-Manager/blob/master/Bcat%20Manager/Bcat%20Manager/BCAT.cs
- https://github.com/Random06457/BCAT-Manager/blob/master/Bcat%20Manager/Bcat%20Manager/NewsCache.cs

This is historical third-party decoder evidence, not a captured response
from the owner's current firmware. It establishes a concrete historical
catalog format incompatible with the current JSON placeholder. No ready-made
catalog dump was located in the searched documentation or this repository's
file inventory. No code, key constants, article text or assets were copied.
A modern sample/hardware check must still validate compatibility. Decryption
support does not supply a signing key or establish console signature acceptance.

## Prepared catalog experiment (probe 0.13.0)

`cmd/newsfixture -catalog-dir <dir>` now creates original decoded catalog,
detail and icon fixtures using the historical wire fields. The catalog is
a MessagePack array containing one openpak_news channel; detail adds an
empty latest_news_urls array; icon is an original 70x70 JPEG. These are
**not** signed/encrypted BCAT containers. They are not mounted on the live
CDN routes. A conforming envelope and hardware acceptance remain unresolved.
Generate reproducibly from the news module:

```sh
go run ./cmd/newsfixture -published 2026-09-08T21:53:14Z -catalog-dir /tmp/openpak-news-catalog
```

Probe 0.13.0 X calls the native news:v downloader (30500) for the catalog URL
already used by Find Channels. It passes a 4 MiB transfer-memory buffer,
waits on the download context's completion event, logs command results and
decoded size, and exports catalog-v013.msgpack only for a successful nonzero
bounded result. This is a diagnostic of the current server response, not a
claim that the generated fixture or current response has been accepted.
The wrapper follows Switchbrew's unofficial command descriptions. Creation
or subcommand failures must first be distinguished from wrapper/version
mismatch; they alone cannot establish signature rejection.

The v0.12 live and expired payloads remain unchanged, so existing targeted
cleanup remains available. The new X action does not post content or change
subscriptions. No nx-baas source or hostname configuration was modified.
Go vet/race tests and pinned NRO compilation pass; hardware execution is next.

## Native downloader result on hardware (0.13.0)

The owner's report starts the 0.13.0 run at epoch 1788906259 (2026-09-08
22:24:19 UTC). news:v, transfer memory, command 30500 and completion-event
creation succeed. The completion event is signaled within the wait.
GetResult returns 0x0000A27D (News module 125, description 81); GetSize
succeeds with decoded size zero. Memory closes cleanly and no catalog is
exported. Existing News count remains 14 at startup; expired-test removal
is still not established.

The remote journal records catalog GETs with HTTP 200 around this run.
A new read-only server check still returns 37-byte JSON (topics, updatedAt),
not a BCAT container. Together these establish that the downloader path is
accessible and this experiment yields no decoded catalog. Public sources
searched did not establish a symbolic meaning for description 81; do not
label it an RSA-signature error or assert which specific parser check failed.

The next discriminating network test needs a known-valid BCAT catalog as a
control, then an original catalog with the required envelope. Current decoded
fixtures alone are insufficient. An unauthenticated, normally verified HTTPS
attempt to retrieve the public upstream catalog failed with URLError and did
not produce a sample. No console credentials were forwarded upstream.

## Read-only News cache inventory (probe 0.14.0)

After 0.13.0 returned News 125-81 with zero decoded bytes, 0.14.0 X inspects
only News system save 0x8000000000000090 via libnx filesystem read APIs.
It tries System then User save space, lists bounded directory entries, and
exports bounded files whose names suggest catalog/topic/detail metadata.
It excludes names indicating keys/secrets/tokens/passphrases, never modifies
the save and does not stop News to force access. A locked/inaccessible save
is reported as a result code. Catalog availability remains unknown. A path
heuristic can miss a cache with an unrelated name; the inventory can guide
the next read without assuming the two article-derived channels are a catalog.
Output is report.txt plus cache-v014-NNN.bin, kept outside repositories.
ZL retains the previous downloader test and A retains existing targeted expiry
cleanup. B still posts the previous bounded test and is not part of this run.
Pinned compilation passes with warnings as errors; hardware result is pending.
