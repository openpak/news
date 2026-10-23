# Creating OpenPak channels and News articles

**Current deployment (2026-09-10):** owner-authorized v006/cdn-v003 now serves four channels, populated archives, twelve summaries and linked full articles. Host and public HTTPS verification pass; native rendering and automatic subscription delivery remain pending. See [current delivery, evidence and rollback](native-news-article-delivery.md). Earlier empty-archive/preparation statements below are historical.

Updated 2026-09-10; hardware evidence through 2026-09-09, HOS 22.5.0 with
Atmosphere on the owner's emuMMC.
Start here for current behavior; the older spike documents are historical.

## What works and what remains

| Capability | Evidence / limit |
| --- | --- |
| Local article title, body and images | Owner confirmed v0.12 NORMAL article via PostLocalNews, associated with `nx_news` |
| Native encrypted/signed catalog download | v0.19 ZL returned success and 155 decoded bytes after both patches and reboot |
| UI follows OpenPak catalog links | Server received OpenPak icon and detail requests; logs alone do not prove rendering |
| Channel archive endpoint | v004 deployed with an empty archive inferred from the 22.5 parser; UI retest pending |
| Channel lineup (four channels) | Sources in `channels/`, multi-channel fixture CDN built and host-validated 2026-09-10; deployment and hardware tests pending owner review |
| Subscribe and automatic delivery | Not yet demonstrated |
| Server-delivered article | Not yet demonstrated; local posting success is a different path |
| Cleanup | Verified expired test later disappeared, count 14→13; no general immediate-delete guarantee |

The current service is a fixed-fixture experiment. **Update 2026-09-23:**
`newscdn -data` now has a channel and article CRUD API with server-side
publishing ([contract](admin-api.md)). It is committed but not deployed.
The original server-only NO-GO still applies to unpatched consoles.

## Create an offline channel and article

Use [the editable sample](../examples/native-news/README.md) to learn the
format, or edit the real [channel lineup](../channels/README.md) with
`cmd/newsseed`, which builds all four OpenPak channels from per-article JSON
files. For a quick single-channel experiment: edit the two JSON files, then
run the sample builder. Multiple channels need a stable `topic_id` each,
their records combined into one catalog array, and each channel's detail,
icon and archives served at its matching path — that dynamic routing now
exists in `internal/newsexperiment` and is host-validated; deploying it is
pending owner review of the seed content.

Channel record fields currently used:

| Field | Type / meaning |
| --- | --- |
| `topic_id` | Stable string used in paths and article association |
| `name`, `publisher`, `description` | Display strings |
| `publishing_time`, `last_posted_at` | Integer UTC Unix seconds |
| `important` | Boolean |
| `shop_query` | Optional; detail only. The raw eShop query behind the channel page's shop button |

In `channels/lineup.json`, a channel can also set `application_ids`: title
ids (16 lowercase hex digits) whose launch auto-subscribes the console to
the channel (`titles/<tid>/topics`). It can also set `test_distribution`,
which makes the module ignore its received-id history. Use that for test
channels only.

The catalog is an array of records. Detail is the same record plus the
optional `shop_query`. `latest_news_urls` is no longer sent: 22.5.0 parses it
nowhere. Archives is a map with boolean `na_required` and array `data_list`
(qlaunch's channel page). The per-topic **list** is what the News module
itself receives; its schema is in
[the delivery doc](native-news-article-delivery.md#reception-path-in-2250-2026-09-23).
Do not invent URL fields: trace their parser first.

The local article format that rendered correctly is `version.format=1`,
`version.semantics=1`, `display_type="NORMAL"`, with `subject` and `body` maps.
Use `body.text`, binary `body.main_image`, and integer `body.main_image_height`.
Use binary `topic_image` and `list_image` too. Records carry priority 100:
qlaunch's main list shows only `50 < priority < 1000`.

An article can end with a link button, `more`. Set it in the article JSON:

```json
"more": {"type": "game", "application_ids": ["0100abcd12340000"]}
"more": {"type": "shop", "query": "dst_nsuid=70010000000001"}
"more": {"type": "browser", "url": "https://openpak.example/guide"}
```

`game` needs 1–5 title ids, `shop` a query (or title ids), and `browser` an
https URL of at most 256 bytes. Only the fields you set are emitted. The key
names are read from 22.5.0 qlaunch; which fields each type uses is inferred
and unverified on hardware (see [provenance](provenance.md)). The earlier LIST/body-array
experiments displayed titles without working body/images; do not treat those
historical fixtures as the current example. The sample preserves the working
layout but uses an OpenPak channel ID whose article association is still untested.

## The OpenPak channel lineup

The real channel set lives in [`channels/`](../channels/README.md):
`openpak_news`, `openpak_games`, `openpak_workshop` and `openpak_help`, each
with three opening articles written from verified evidence. `cmd/newsseed`
builds decoded payloads, `tools/news-cdn/sign-seed.sh` signs them with the
owner-private keys and verifies every container against the public key.
The full inventory, commands, update/retire behaviour and the remaining
hardware checks are documented there. The CDN derives its per-topic routes
from the signed fixture tree, so a channel appears by adding its fixtures —
but the catalog is never decoded server-side (the CDN holds no keys), so
the seed tool is what keeps catalog and routes consistent.

## Wrap decoded files for the console

Native News expects a BCAT container, not JSON or bare MessagePack. The tested
container has a 0x120-byte header, an RSA-2048 PKCS#1 v1.5 SHA-256 signature over
plaintext and AES-CTR encrypted payload. IVs are freshly generated. The current
News AES profile and index 0 produced a successful native catalog decode.
This does not establish game BCAT AES keys or other indices.

The generator accepts a prederived raw AES key and an OpenPak RSA private key.
For the owner's existing patch, use the existing matching key; generating a new
key requires updating the console's public-key patch. Never commit keys,
firmware, saves, certificates containing device identity or console exports.

From the `news` repository, after building the example:

```sh
sample_dir=/tmp/openpak-news-sample
signed_dir=/tmp/openpak-news-sample-signed
news_private=~/REPOS/Openpak/.secrets/news
mkdir "$signed_dir"
for name in catalog detail archives article; do
  go run ./cmd/newscontainer \
    -in "$sample_dir/$name.msgpack" -out "$signed_dir/$name.bcat" \
    -aes-key-file "$news_private/news-aes-256-candidate.key" \
    -signing-key-file "$news_private/openpak-bcat-signing.pem" -secret-index 0
done
go run ./cmd/newscontainer \
  -in "$sample_dir/icon.jpg" -out "$signed_dir/icon.bcat" \
  -aes-key-file "$news_private/news-aes-256-candidate.key" \
  -signing-key-file "$news_private/openpak-bcat-signing.pem" -secret-index 0
```

These are owner-local paths, not credentials bundled with the example. Outputs
must not exist. Deploy only ciphertext fixtures to the CDN; it needs no content
signing key or AES key. Signing the article above only prepares a file: the
current server has no article route or archive entry linking to it.

## Console-facing routes

All implemented routes are GETs on `bcat-topics-lp1.cdn.nintendo.net`.
`<topic>` is one of the stable topic ids the catalog lists (today the four
OpenPak channels); unknown topics, including Nintendo's own, fall through
to the adapter fallback as before:

| Path | File in the signed fixture tree |
| --- | --- |
| `/api/nx/v1/topics/catalog` | `catalog.bcat` (one container, all channels) |
| `/api/nx/v1/topics/<topic>/detail` | `<topic>/detail.bcat` |
| `/api/nx/v1/topics/<topic>/icon` | `<topic>/icon.bcat` (JPEG plaintext) |
| `/api/nx/v2/topics/<topic>/online_archives` | `<topic>/archives.bcat` |
| `/api/nx/v1/list/<topic>` | `<topic>/list.bcat`: what the News module receives. Reached through the nx-baas forward from `bcat-list-lp1` |
| `/openpak/v1/topics/<topic>/news/<id>/{summary,article}` | `<topic>/<id>/{summary,article}.bcat` |
| `/openpak/v1/topics/<topic>/news/<id>/expired` | `retired/<topic>/<id>/article.bcat` |
| `/api/nx/v1/titles/<tid16>/topics` | `titles/<tid>.bcat`, else `titles/none.bcat` |
| `/api/nx/v2/latest_news/<n>` | the newest article's summary |

The v1 `topics/<topic>/archives` and `topics/<topic>/<id>/article` routes
still answer, but 22.5.0 builds neither path. Every response carries a strong
ETag and answers If-None-Match (304) and Range/If-Range.

The fixed fixtures ignore query parameters. A production renderer must honor
language/country targeting and any verified pagination behavior. Responses use
`application/octet-stream`, accurate lengths, an ETag and
`Cache-Control: no-store` for testing. Unknown paths currently forward to the existing adapter; that is not
proof the fallback returns a native-compatible container.

Local smoke test of signed fixtures (the seed's signed tree works directly):

```sh
go run ./cmd/newscdn -dir /tmp/openpak-news-sample-signed -listen 127.0.0.1:20040
```

This is plain HTTP for host inspection. Console deployment needs the existing
TLS/SNI arrangement; do not point the console at this plain HTTP listener.

## Hardware setup and validation

The owner has installed two separate patches for the pinned 22.5 BCAT build:

1. [Verification key](../tools/news-patch/README.md): shared retail key replacement,
   affecting News and game BCAT verification. Probe **R/L** install/remove it.
2. [News token omission](../tools/news-patch/news-no-dauth/README.md): bypasses the
   pre-download edge-token request in the News memory worker. Probe **D-pad
   Right/Left** install/remove it. It applies to all URLs using that path.

Both require rebooting Atmosphere after a change and apply to matching boots
sharing the SD, not intrinsically only emuMMC. The OpenPak installer must redirect
the exact topics hostname; keep OpenPak DNS enabled. Do not change the global
dauth service or request a new device certificate for this workflow.

Probe v0.19 **ZL** tests native catalog download/decode, without subscribing or
posting. Success exports `sd:/switch/openpak-news-probe/catalog-v013.msgpack`;
the filename is historical. Compare its bytes with the source catalog before
claiming plaintext equality. Then test News → Find Channels → OpenPak News.
Correlate catalog, detail, icon and archives requests with the UI result.
After channel loading works, explicitly test subscription persistence through
reboot, one fresh server-delivered article, an update and scoped retirement.

Do not repeatedly press **B**: that is the old local posting experiment. Use
stable IDs for updates, a fresh ID for a new trial, and verify the exact stored
record before cleanup. PostLocalNews success alone has not guaranteed storage.
Server unpublishing alone does not prove removal of the console's cached article.

## Existing platform-neutral authoring

Production content belongs in `internal/content`, `internal/store` and the
versioned `proto/news/v1/news.proto` contract. `cmd/newsctl` already supports
article create/publish/list/preview and image upload. It does not create native
channels or populate the experimental fixture CDN. Proposed channel contracts
and the renderer bridge are implementation work, not available commands.

For an already running local authoring service with `NEWS_OPERATOR_KEY` supplied
through its environment, the actual CLI syntax is:

```sh
go run ./cmd/newsctl -addr localhost:7001 article create \
  -slug openpak-welcome -platforms switch -region GB \
  -locale 'en-GB=Welcome to OpenPak' -body-text 'News from the OpenPak network.'
# Use the ID returned by create:
go run ./cmd/newsctl -addr localhost:7001 article publish -id ARTICLE_ID
```

Use the real configured address; 7001 is the CLI default, not a verified remote
deployment port. The CLI uses insecure gRPC: use a local service or approved
private tunnel, not an exposed public listener. These commands mutate the
authoring service when run; the offline sample does not.

See the [agent handoff](native-news-agent-handoff.md) for the implementation
sequence, deployment locations, acceptance checks and a ready-to-use task.
