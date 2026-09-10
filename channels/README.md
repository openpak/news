# OpenPak channel lineup seed

The editable source of the OpenPak News channels for the native Switch News
experiment: four channels and their opening articles. Everything here is
original — text written from our own evidence, artwork drawn by
[`cmd/newsseed`](../cmd/newsseed) from geometric motifs. No Nintendo artwork,
no third-party game assets, no keys.

This is the production-shaped input for the experimental CDN. It respects the
platform-neutral architecture: the payload shapes live in
[`internal/newsformat`](../internal/newsformat) (reusable by a future
content-store renderer), and this seed is file-based authoring until native
channel contracts exist (none do today — see the
[handoff](../docs/native-news-agent-handoff.md)).

## Status: prepared, reviewed locally, not deployed

What hardware has already proven (evidence in
[`docs/native-news-container.md`](../docs/native-news-container.md)):

| Capability | Evidence |
| --- | --- |
| Local article renders natively (title, body, images) | Probe v0.12, owner-confirmed, payload matched byte-for-byte |
| Native catalog download + decode of an OpenPak-signed container | Probe v0.19 ZL, 155 decoded bytes, 2026-09-09 |
| Console requests per-channel detail/icon/archives | Server logs 18:51, 20:14–20:15 UTC |
| Empty-archives channel response shape (`{na_required, data_list}`) | v004 fixture built from the 22.5 parser; **UI acceptance still pending** |

What this seed adds, pending owner review and hardware validation: a
four-channel catalog, per-channel signed fixtures on dynamically derived
routes, and the article payload set for the delivery-path work. **No article
is served or delivered yet**: the archive `data_list` entry format has not
been traced, so every archives response stays empty and the signed article
containers are prepared artifacts only. Nothing in this tree claims to be on
a console.

## Layout

```
channels/
  lineup.json                     channel identities (stable topic ids, icons)
  retired.json                    scoped retirement list (empty)
  openpak_news/<news_id>.json     one file per article, named by its id
  openpak_games/<news_id>.json
  openpak_workshop/<news_id>.json
  openpak_help/<news_id>.json
```

**Topic ids are permanent.** They appear in catalog records and console
request paths: `openpak_news`, `openpak_games`, `openpak_workshop`,
`openpak_help`. Changing one is a breaking change, not an edit.

**News ids are permanent.** Scheme: `YYMMDD` of the day the article joined
the lineup + a 4-digit sequence (`2609100001` = News' first article). Keep
under 2^32, never reuse an id — including after retirement. An expired
identity can suppress reinsertion of the same id (observed in probe 0.12),
and stable ids are what make scoped update-in-place possible.

Article file fields: `title` (≤ 100 chars), `body` (**one paragraph**,
≤ 1200 chars — newline rendering is unverified in the native reader),
`footer` (≤ 80), `language` + `supported_languages` (`en-GB` style, the
article language must be listed), `lifetime_days` (1..30), `art`
(`motif` + `hue` 0..359). Motifs: `beacon`, `pad`, `gear`, `buoy`, `waves`,
`rays`, `orbit`, `grid`, `steps`, `spark`.

## Build

Deterministic: with `--published` pinned, a rerun is byte-identical, and
stale artifacts of retired/renumbered articles are pruned — a rerun updates
existing seed content in place, never duplicating.

```sh
go run ./cmd/newsseed --list                                   # inventory
go run ./cmd/newsseed --out /tmp/openpak-news-seed \
    --published 2026-09-10T08:00:00Z                           # build
```

Outputs, relative to the out dir: `catalog.msgpack` (all four channel
records), `<topic>/detail.msgpack`, `<topic>/icon.jpg` (70×70),
`<topic>/archives.msgpack`, `<topic>/<id>/{article.msgpack,card.jpg,hero.jpg,
article-expired.msgpack}` (card 358×201, hero 800×364, embedded in the
article too), `retired/<topic>/<id>/article.msgpack` (expired replacement
for anything retired), and `manifest.json` (inventory + SHA-256 of every
payload). Requires libjpeg-turbo's `cjpeg` for the baseline JFIF 4:4:4
images the console has accepted in every rendering test.

## Sign (keys stay off the repository)

```sh
tools/news-cdn/sign-seed.sh -i /tmp/openpak-news-seed \
  -o /tmp/openpak-news-seed-signed \
  -k ~/openpak-news-private/openpak-bcat-signing.pem \
  -a ~/openpak-news-private/news-aes-256-candidate.key
```

The script builds `cmd/newscontainer`, wraps every payload into a BCAT
container (RSA-2048 PKCS#1 v1.5/SHA-256 over plaintext, AES-CTR, fresh IV,
secret index 0), then **verifies each container independently with openssl
against the public key in `tools/news-patch/openpak-bcat-public.pem`** and
writes `SHA256SUMS`. It skips `retired/` and `manifest.json`. The signing
and AES key paths above are owner-local; never copy them into any
repository, and never print their contents. Rollback of a signing run is
just deleting the signed directory.

## Serve and inspect locally

```sh
go run ./cmd/newscdn -dir /tmp/openpak-news-seed-signed -listen 127.0.0.1:20040
curl -s http://127.0.0.1:20040/api/nx/v1/topics/catalog -o /tmp/cat.bcat
```

Routes derive from the fixture tree — `catalog` at
`/api/nx/v1/topics/catalog`, plus per channel `.../topics/<topic>/detail`,
`.../topics/<topic>/icon`, and `/api/nx/v2/topics/<topic>/online_archives`.
This listener is plain HTTP for host inspection only; the console always
uses the TLS arrangement on the server. Unknown topics (including
Nintendo's own channels) fall through to the adapter fallback exactly as
before.

## Deploy (only after owner review of the signed content)

The deployed experiment is image v004 with the old single-channel fixture
directory. Moving to this seed is a new tag and a new fixture directory,
prepared before activation:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
  -o tools/news-cdn/newscdn ./cmd/newscdn
cd tools/news-cdn && podman build -t localhost/openpak-news-experiment:v005 . && cd ../..
ssh <prod-host> 'mkdir -p ~/openpak/deploy/news-experiment/cdn-v002'
scp -r /tmp/openpak-news-seed-signed/* <prod-host>:~/openpak/deploy/news-experiment/cdn-v002/
```

Then, on the server, edit
`~/.config/containers/systemd/openpak-news-experiment.container` in two
places — `Image=...:v005` and the fixtures volume `cdn-v001` → `cdn-v002` —
and `systemctl --user daemon-reload && systemctl --user restart
openpak-news-experiment`. Keep the v004 image for rollback (revert both
lines, restart). Public HTTPS bytes can be checked with the existing CA.
Nothing above has been run for this seed; the deployed service is still
v004 until the owner acts.

## Update and retire

- **Update an article:** edit its JSON, rerun build + sign. Same `news_id`,
  new timestamps and content — the identity the console knows stays stable.
- **Retire one article:** `go run ./cmd/newsseed --retire openpak_help/2609100303`
  (add a real `reason` to `channels/retired.json`), then rebuild and sign.
  The article leaves the build; an expired replacement with the same id is
  produced under `retired/` for the delivery path once it exists. Retirement
  on the console is expiry-based — the expired card disappears during the
  console's own housekeeping (observed once, 14→13 records); instant
  deletion is **not** guaranteed and not promised anywhere.
- **Un-retire:** `--unretire topic/id`, rebuild, resign.
- **Remove a channel:** retire all its articles and drop it from
  `lineup.json`; the next catalog simply stops listing it.

## Inventory (2026-09-10 seed)

| Topic | Name | Icon |
| --- | --- | --- |
| `openpak_news` | OpenPak News | beacon, blue |
| `openpak_games` | OpenPak Games | d-pad cross, green |
| `openpak_workshop` | OpenPak Workshop | hex hub, violet |
| `openpak_help` | OpenPak Help | lifebuoy, orange |

| Article | Title | Evidence it rests on |
| --- | --- | --- |
| `openpak_news/2609100001` | Welcome to the OpenPak channel lineup | this lineup; catalog decode evidence |
| `openpak_news/2609100002` | OpenPak content decodes on your console | probe v0.19 ZL (2026-09-09); v0.12 local rendering |
| `openpak_news/2609100003` | The network behind the news | OpenPak README network milestone; `status/` status page live |
| `openpak_games/2609100101` | Outbound players share a world on OpenPak | `servers/outbound/HANDOFF.md` (2026-09-06/07: shared world, stock-flow console link) |
| `openpak_games/2609100102` | Stardew Valley farms go multiplayer | `servers/stardew-valley/README.md` (2026-09-07, Ryujinx-verified) |
| `openpak_games/2609100103` | How a game earns its place here | MK8D/SSBU "never tested against a console"; Splatoon 3 early research |
| `openpak_workshop/2609100201` | Anatomy of a signed News container | `internal/bcatcontainer`, Switchbrew BCAT container format |
| `openpak_workshop/2609100202` | How your console finds our channels | server request logs; v004 archives work in `docs/native-news-container.md` |
| `openpak_workshop/2609100203` | Two small patches, honestly explained | `tools/news-patch/README.md`, `tools/news-patch/news-no-dauth/README.md` |
| `openpak_help/2609100301` | Getting your console ready for OpenPak News | installer hosts fix (probe v0.16), patch installers R/L and D-pad |
| `openpak_help/2609100302` | When News will not load, read this | documented 0x167B history: DNS → TLS profile → edge token |
| `openpak_help/2609100303` | Where articles live, and when they leave | verified expiry replacement + later disappearance; no instant-delete guarantee |

## Remaining hardware checks (in order)

1. **Catalog with four channels** decodes natively. Only the one-channel,
   155-byte catalog is hardware-proven; this catalog decodes to 720 bytes
   host-side. No size limit is documented, but it is a new payload.
2. **Channel page acceptance** — still pending from v004. Open News →
   Find Channels, expect the four OpenPak channels with distinct icons.
   Correlate catalog/detail/icon/archives requests in the service log.
3. **Per-channel detail + icon** render when each channel is opened.
4. **Subscription** on one channel, persistence across reboot.
5. **Article delivery** — blocked on the archive `data_list` entry format
   (trace the parser at the documented qlaunch offsets first; do not guess
   fields). The signed article containers here are ready inputs for that
   work, not deliverable fixtures.
6. **Update + scoped retirement** once delivery works: replace one
   article's bytes by id, then retire one and confirm the others are
   untouched.
