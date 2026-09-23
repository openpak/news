# OpenPak channel lineup seed

**Current deployment (2026-09-10):** owner-authorized v006/cdn-v003 now serves four channels, populated archives, twelve summaries and linked full articles. Host and public HTTPS verification pass; native rendering and automatic subscription delivery remain pending. See [current delivery, evidence and rollback](../docs/native-news-article-delivery.md). Earlier empty-archive/preparation statements below are historical.

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

## Status

**Deployed:** v006/cdn-v003 (publication pinned 2026-09-10T19:13:51Z) serves
the four-channel catalog, populated archives, twelve summaries and linked
full articles. See [delivery, evidence and rollback](../docs/native-news-article-delivery.md).

**Local, pending owner review:** the 2026-09-11 artwork rebrand — channel
icons rebuilt from the official OpenPak mark and per-article scene banners
(the previous abstract shapes are what v006 currently serves). Built and
signed locally; not in any image on the server. Deploying it is the next
image/fixtures round (v007/cdn-v004) using the documented procedure.

What hardware has already proven (evidence in
[`docs/native-news-container.md`](../docs/native-news-container.md) and
[`docs/native-news-article-delivery.md`](../docs/native-news-article-delivery.md)):

| Capability | Evidence |
| --- | --- |
| Local article renders natively (title, body, images) | Probe v0.12, owner-confirmed, payload matched byte-for-byte |
| Native catalog download + decode of an OpenPak-signed container | Probe v0.19 ZL, 155 decoded bytes, 2026-09-09 |
| Console requests per-channel detail/icon/archives | Server logs 18:51, 20:14–20:15 UTC |
| Archive entry schema (news_id, version, default_language, languages[].summary_url) | Owner-local qlaunch parser trace, 2026-09-10; served by v006, **console acceptance pending** |
| Summary card + full-article routes | Served by v006; **native rendering pending** |

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
(`motif`) and an optional `more` link button: `{"type": "game",
"application_ids": ["<tid16>"]}`, `{"type": "shop", "query": "…"}` or
`{"type": "browser", "url": "https://…"}` (see the
[authoring guide](../docs/native-news-authoring-guide.md)).

Optional article fields (the admin API writes them; see
[`docs/admin-api.md`](../docs/admin-api.md)): `published_at` (RFC 3339 UTC;
absent = the build's `--published`), `priority` (51..1998, default 100),
`pickup_days` (0..`lifetime_days`, default until expiry) and
`essential_pickup` (`{"pickup_days", "priority_after"}`, default none). A
file without them builds byte-identically to before. An uploaded channel
icon at `icons/<topic_id>.png|jpg` replaces the motif icon. `screenshots`
and `icons` are reserved topic ids.

Optional channel fields in `lineup.json`: `shop_query` (the channel page's
eShop button), `application_ids` (titles whose launch auto-subscribes the
console to the channel) and `test_distribution` (bypasses the console's
received-id history; test channels only). Topic ids `retired` and `titles`
are reserved.

**Real screenshots** (PRD rectified 2026-09-11): drop an operator-captured
gameplay image at `channels/screenshots/<news_id>.png` (or `.jpg`) and it
replaces the generated banner for that article — center-cropped and
re-encoded to the exact card/hero sizes automatically. Only Outbound
(`2609100101`, `2609110101`) carries one today. Captures go to the citron
screenshot folder (`~/.local/share/citron/screenshots`, named by title id)
or any capture tool — copy one over, rename to the article id, rebuild,
resign. The game must be credited in the article body, and any image comes
down on the rights holder's request (see the PRD's 2026-09-11 amendment).
Without a screenshot, the generated scene art is used.

Article motifs are gaming scenes built on the official mark:
`mark`, `burst`, `horizon`, `pad`, `levels`, `podium`, `gear`, `orbit`,
`wrench`, `power`, `question`, `refresh`, plus per-title scenes — `globe`
(Outbound), `farm` (Stardew Valley), `pod` (Among Us), `flag`
(Mario Kart 8 Deluxe), `clash` (Smash Bros.), `splatduo`/`splat`
(Splatoon 3/2), `island` (Animal Crossing), `spring` (ARMS), `mansion`
(Luigi's Mansion 3), `golf`, `dice` (Party Superstars), `strike`
(Strikers), `tennis` (Tennis Aces), `claws` (Monster Hunter GU), `block`
(Super Mario Bros. 35) — and `relay`/`beacon` for the console-network
pieces. Channel icons reproduce the OpenPak logo (`mark`) or swap its
centre dot for a glyph: `mark-pad`, `mark-gear`, `mark-question`.

## Build

Deterministic: with `--published` pinned, a rerun is byte-identical, and
stale artifacts of retired/renumbered articles are pruned — a rerun updates
existing seed content in place, never duplicating. Pruning is conservative:
the build drops a `.newsseed-managed` marker and prunes only paths recorded
in the previous `manifest.json` (plus its own `retired/` tree). A directory
newsseed did not create is never modified — still, give `--out` a directory
of its own. A directory from before markers existed (or a foreign one) is
rejected outright: use a fresh `--out`, or pass `--adopt` once to migrate
it, which sweeps its previously generated seed artifacts (known names only)
before rebuilding.

```sh
go run ./cmd/newsseed --list                                   # inventory
go run ./cmd/newsseed --out /tmp/openpak-news-seed \
    --published 2026-09-10T08:00:00Z                           # build
```

Outputs, relative to the out dir: `catalog.msgpack` (all four channel
records), `<topic>/detail.msgpack`, `<topic>/icon.jpg` (70×70),
`<topic>/archives.msgpack` (populated: one entry per active article,
newest first, linking its summary), `<topic>/<id>/{summary.msgpack,
article.msgpack,card.jpg,hero.jpg,article-expired.msgpack}` (summary card
title + 70×70 + 358×201 + article URL; card and hero also embedded in the
article), `<topic>/list.msgpack` (the bcat-list the News module receives:
one entry per active article linking its article container with the exact
signed size, plus each retired article's expired replacement with
`overwrite`), `retired/<topic>/<id>/article.msgpack` (expired replacement
for anything retired), `retired/<topic>/list.msgpack` (a removed channel's
list, `service_status: "expired"`), `titles/<tid>.msgpack` and
`titles/none.msgpack` (auto-subscription topic arrays), and
`manifest.json` (inventory + SHA-256 of every payload, plus each article's
content hash and overwrite flag). Requires libjpeg-turbo's `cjpeg` for the baseline JFIF 4:4:4
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
writes `SHA256SUMS` (excluding itself). It signs everything except
`manifest.json`, including `retired/` and `titles/`, and checks that each
container is exactly 288 bytes plus its payload: the size the lists
advertise. Input, output and key paths must not overlap — a `-f` run
deletes the output directory first, so an output that contains the input or
a key is rejected before anything is removed. The signing
and AES key paths above are owner-local; never copy them into any
repository, and never print their contents. Rollback of a signing run is
just deleting the signed directory.

Routes derive from the fixture tree — `catalog` at
`/api/nx/v1/topics/catalog`, plus per channel `.../topics/<topic>/detail`,
`.../topics/<topic>/icon`, `/api/nx/v2/topics/<topic>/online_archives`, and
the per-article `/openpak/v1/topics/<topic>/news/<id>/{summary,article}`,
`/api/nx/v1/list/<topic>`, `/openpak/v1/topics/<topic>/news/<id>/expired`,
`/api/nx/v1/titles/<tid>/topics` and `/api/nx/v2/latest_news/<n>`.
This listener is plain HTTP for host inspection only; the console always
uses the TLS arrangement on the server. Unknown topics (including
Nintendo's own channels) fall through to the adapter fallback exactly as
before.

## Deploy

The live deployment is v006/cdn-v003 (see
[the delivery doc](../docs/native-news-article-delivery.md) for its exact
facts and rollback). The artwork rebrand is **not deployed**; after owner
review it ships as the next round (v007/cdn-v004) with the same procedure:
cross-build the arm64 binary, build the image on the server, copy the signed
fixture tree, then switch the quadlet's `Image=` and fixtures volume together
and restart. Keep the previous image and fixture directory for rollback.

## Update and retire

- **Update an article:** edit its JSON, rerun build + sign **into the same
  `--out`**. Same `news_id`; the build compares the content hash with the
  previous `manifest.json` and sets the list's `overwrite` from then on. A
  console skips a news_id it has already received unless `overwrite` is set,
  so an update built into a fresh directory would be ignored by consoles
  that hold the old copy.
- **Retire one article:** `go run ./cmd/newsseed --retire openpak_help/2609100303`
  (add a real `reason` to `channels/retired.json`), then rebuild and sign.
  The article loses its archive link and active outputs; an expired
  replacement with the same id is produced under `retired/` and listed with
  `overwrite`, so the console replaces its copy. Retirement on the console is expiry-based — the
  expired card disappears during the console's own housekeeping (observed
  once, 14→13 records); instant deletion is **not** guaranteed and not
  promised anywhere.
- **Un-retire:** `--unretire topic/id`, rebuild, resign. (This restores the
article to an active channel; a removed channel must first be restored to
`lineup.json`.)
- **Remove a channel:** retire its articles first (see above). Then, for
each retirement entry, copy the channel's block from `lineup.json` into the
entry as its `topic` snapshot, and delete the channel's block from
`lineup.json`. Keep the article source files — they remain the payload
source for the retirement fixtures, which the build keeps producing under
`retired/` and records under `removed_channels` in the manifest. The next
catalog simply stops listing the channel, and its list answers
`service_status: "expired"`, which retires the topic on consoles still
subscribed to it. Rebuilding without the topic
snapshot (or with a removed-channel article still unretired) fails with an
explanation, never silently. Removing the **last** channel builds an empty
catalog, and the CDN serves it as catalog-only — a deliberate state, though
its Find Channels behaviour on hardware is untested; prefer keeping at
least one channel live.

## Inventory (2026-09-11 seed — 33 articles)

| Topic | Name | Icon (official mark, centre dot swapped) |
| --- | --- | --- |
| `openpak_news` | OpenPak News | the official logo, unchanged |
| `openpak_games` | OpenPak Games | centre dot replaced by a gamepad |
| `openpak_workshop` | OpenPak Workshop | centre dot replaced by a gear |
| `openpak_help` | OpenPak Help | centre dot replaced by a question mark |

| Article | Title | Evidence it rests on |
| --- | --- | --- |
| `openpak_news/2609100001` | Welcome to the OpenPak channel lineup | this lineup; catalog decode evidence |
| `openpak_news/2609100002` | OpenPak content decodes on your console | probe v0.19 ZL (2026-09-09); v0.12 local rendering |
| `openpak_news/2609100003` | The network behind the news | OpenPak README network milestone; `status/` status page live |
| `openpak_news/2609110001` | The Switch is OpenPak's home network | retail-Switch milestone (main README); servers roster |
| `openpak_news/2609110002` | The Wii U network takes shape | `nn-account` M2/M3 done; `nn-friends` M3 end-to-end vs live core |
| `openpak_news/2609110003` | The 3DS shares the new network brain | same adapter family; `nn-account`/`nn-friends` status |
| `openpak_news/2609110004` | The Wii: planned, honest, not yet built | `research-third-party-networks.md` verdicts (Wiimmfi closed; WiiLink WFC open) |
| `openpak_news/2609110005` | Nintendo DS: the plan is on file | same research: one GameSpy-style base planned for Wii+DS |
| `openpak_games/2609100101` | Outbound players share a world on OpenPak | `servers/outbound/HANDOFF.md` (2026-09-06/07) |
| `openpak_games/2609100102` | Stardew Valley farms go multiplayer | `servers/stardew-valley/README.md` (2026-09-07, Ryujinx-verified) |
| `openpak_games/2609100103` | How a game earns its place here | server status lines across `servers/` |
| `openpak_games/2609110101` | Outbound is open — come share a world | playable: shared world on emulators; console link done |
| `openpak_games/2609110102` | Stardew Valley farms are live on OpenPak | playable: 3-player farms verified 2026-09-07 |
| `openpak_games/2609110103` | Among Us gets a self-hosted home | deployed to production 2026-09-11; console-untested |
| `openpak_games/2609110104` | Mario Kart 8 Deluxe: the pit lane is busy | builds/listens, never console-tested; known kerberos key-size gap |
| `openpak_games/2609110105` | Smash Bros. Ultimate enters the workshop | runs, DataStore round-trips vs real Postgres |
| `openpak_games/2609110106` | Splatoon 3 research is open and early | NPLN: 4 services answered, rotation generator, never vs retail |
| `openpak_games/2609110107` | Splatoon 2's server inked in | NEX: builds/listens/schemas, never console-tested |
| `openpak_games/2609110108` | Animal Crossing's island joins the programme | runs vs real Postgres + shared libraries |
| `openpak_games/2609110109` | ARMS extends into the roster | runs, healthz 22142, WebSocket listeners |
| `openpak_games/2609110110` | Luigi's Mansion 3: the lights are on | runs, auth/secure 22190/22191 |
| `openpak_games/2609110111` | Mario Golf: Super Rush books its tee time | peer-to-peer title: login + lobby list role |
| `openpak_games/2609110112` | Mario Party Superstars rolls the dice | NEX 4.6.5 / Pia 5.33.0 registered |
| `openpak_games/2609110113` | Mario Strikers is mid-match in development | substrate registered; club play capture-driven next |
| `openpak_games/2609110114` | Mario Tennis Aces warms up on the host | builds, healthz, schemas, matchmaking |
| `openpak_games/2609110115` | Monster Hunter Generations Ultimate hunts here | DataStore round-trips vs Postgres |
| `openpak_games/2609110116` | Super Mario Bros. 35: bringing it back | delisted 2021-03-30; every NEX protocol registered |
| `openpak_workshop/2609100201` | Anatomy of a signed News container | `internal/bcatcontainer`, Switchbrew BCAT container format |
| `openpak_workshop/2609100202` | How your console finds our channels | server request logs; `docs/native-news-container.md` |
| `openpak_workshop/2609100203` | Two small patches, honestly explained | `tools/news-patch/README.md`, `tools/news-patch/news-no-dauth/README.md` |
| `openpak_help/2609100301` | Getting your console ready for OpenPak News | installer hosts fix (probe v0.16), patch installers R/L and D-pad |
| `openpak_help/2609100302` | When News will not load, read this | 0x167B history: DNS omission + edge token confirmed; TLS profile never proven necessary |
| `openpak_help/2609100303` | Where articles live, and when they leave | verified expiry replacement + later disappearance; no instant-delete guarantee |

Article banners use per-title scene motifs (`globe`, `farm`, `pod`, `flag`,
`clash`, `splatduo`, `splat`, `island`, `spring`, `mansion`, `golf`, `dice`,
`strike`, `tennis`, `claws`, `block`) and the shared set for the console
pieces (`horizon`, `orbit`, `burst`, `relay`, `beacon`). All art is drawn
by [`cmd/newsseed/art.go`](../cmd/newsseed/art.go) from the official mark's
geometry — original, deterministic, no screenshots or third-party assets
(those are copyrighted material and stay out of the repository by rule).

## Remaining hardware checks (in order)

1. **Populated archive acceptance** — the console parses each channel's
   `data_list` entries (news_id, version, default_language,
   languages[].summary_url) and requests the summaries. Deployed in v006;
   console-side acceptance is the open gate.
2. **Summary card + full article rendering** — title, 70×70 icon and
   358×201 card from the summary, then the NORMAL article through the
   remote reader (layout proven locally in v0.12; remote acceptance
   pending).
3. **Channel page + icons** — four channels listed with distinct icons;
   after the artwork rebrand deploys, re-check the icons specifically.
4. **Subscription** on one channel, persistence across reboot, automatic
   refresh delivery (separate from the archive path; unimplemented).
5. **Update + scoped retirement** — replace one article's bytes by id,
   then retire one and confirm the others are untouched.
