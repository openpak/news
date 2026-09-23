# Channel archive and article delivery experiment

> **2026-09-23:** the 22.5.0 firmware audit replaced the reception model
> below. The News module fetches only the bcat-list and the URLs it names;
> subscription status 2 (not 1) means subscribed. See
> [the reception path](#reception-path-in-2250-2026-09-23) at the end and
> [provenance](provenance.md). The archive/summary sections still describe
> qlaunch's channel pages correctly.

2026-09-10. Owner authorized deployment. Image **v006** now serves four
channels with three articles each on the isolated News CDN. Host and public
HTTPS checks pass; native UI acceptance of populated archives, summary cards
and full articles remains pending. Automatic subscription refresh is separate.

## Traced request chain

Owner-local HOS 22.5 qlaunch parses online_archives as a map containing
`na_required` and `data_list`. Each article entry is generated as follows
(JSON illustration; the actual decoded response is MessagePack):

```json
{
  "news_id": 2609100001,
  "version": {"format": 1, "semantics": 1},
  "default_language": "en-GB",
  "languages": [{
    "language": "en-GB",
    "summary_url": "https://bcat-topics-lp1.cdn.nintendo.net/openpak/v1/topics/openpak_news/news/2609100001/summary"
  }]
}
```

Archive entry parser: news_id at `0x654244`, version at `0x654278`, optional
conditions at `0x654344`, default_language at `0x654400`, languages at
`0x6544a4`. Language members recognize language (`0x654614`) and summary_url
(`0x654550`). Preserve language before summary_url: this streaming parser
selects a URL using the language already read. Serial conditions are omitted
for untargeted original seed articles. The seed emits one en-GB record and
relies on default-language fallback, not actual translated editions.

The summary downloader at `0x67c24c` uses the native memory-download wrapper.
Its parser (`0x67c364` onward) recognizes no_photography, subject.text, movie,
icon_image, list_image and url. Each summary contains the title, binary original
70×70 icon and 358×201 card image, and a URL to the full article. Full articles
retain the NORMAL, version 1/1, body-map layout proven in the local v0.12 test;
remote-reader acceptance still needs hardware verification.

Summary URLs fit the observed 216-byte buffer. Three entries per archive fit
the observed 300-entry storage. Pagination and production locale/country filters
are not implemented. All links stay on the already redirected topics hostname.

## Implemented routes and lifecycle

Existing catalog/detail/icon routes remain. Each active channel now exposes:

- `/api/nx/v2/topics/<topic>/online_archives`: populated archive.
- `/openpak/v1/topics/<topic>/news/<id>/summary`: signed summary card.
- `/openpak/v1/topics/<topic>/news/<id>/article`: signed full article.

The CDN loads paired summary/article containers from numeric article directories;
invalid IDs and missing pairs fail startup. Expired artifacts, source JSON and
arbitrary file paths are not exposed. No signing/AES keys enter the CDN image.
The `latest_news_urls` field in channel detail was sent empty until
2026-09-23; 22.5.0 parses it nowhere, so it is no longer emitted.

`newsseed` writes summary files and archive links for active IDs, newest ID
first. Retirement removes the archive link and active output directory; scoped
expired payloads remain prepared locally. Fixtures are loaded at startup, so
deploy updated signed files with a service restart. Removing server content
does not guarantee removal of previously cached console articles.

## Deployment and validation

Intermediate v005/cdn-v002 deployed the reviewed four-channel empty-archive
seed. v006/cdn-v003 adds delivery. Private local build/sign directories are
`seed-v006` and `signed-v006`; publication time is **2026-09-10T19:13:51Z**.

Active image: `localhost/openpak-news-experiment:v006`, digest
`4b996646cef8798f35e558cdccc543a8be5d5f9281563becbedae0a3c2e64a6a`.
Remote context: `~/openpak/deploy/news-experiment/image-v006`; fixtures:
`~/openpak/deploy/news-experiment/cdn-v003`. Service, TLS and Traefik routing
remain unchanged. Private content keys stayed local.

Validation:

- Full race tests and vet pass; DB integration was not exercised without
  NEWS_TEST_DATABASE.
- Independent host HTTP traversal followed all four archives to twelve summaries
  and full articles, checking AES, RSA, MessagePack, IDs, channel association,
  summary/article title and card-image agreement.
- All 49 containers (including prepared expired copies) passed OpenSSL signature
  verification and remote checksum verification.
- Public HTTPS with CA verification returned exact fixture bytes for all four
  archives and all 24 summary/article resources.
- Regression checks cover retirement removing archive links, paired resources,
  and rejection of unapproved resource paths.

Rollback: set the experimental quadlet image to v005 and volume to cdn-v002,
daemon-reload and restart. v004/cdn-v001 remains available for the original
single-channel test. Preserve unrelated Traefik configuration.

## Firmware-driven reception (2026-09-16)

The missing half of the BCAT route — subscribe, then receive — turns out to
be reachable over IPC rather than only through the Find Channels UI. libnx
`news.h` exposes, and switchbrew documents, four INewsService commands:

| Cmd | Name |
| --- | --- |
| 30100 | GetSubscriptionStatus |
| 30101 | GetTopicList |
| 30300 | RequestImmediateReception |
| 40100 | SetSubscriptionStatus |

Each service exposes a permission mask — news:p 0x1, news:c 0x2, news:v 0x4,
news:m 0xd — and these commands are split across them, so no single
restricted service has all four. news:a is masked 0xffffffff and the probe
already opens it, so it uses that rather than deriving the exact split from
an undocumented bit mapping. So probe **v0.21**
(D-pad Down) does the same for `openpak_news` with full diagnostics —
topic list, per-filter result codes, record count before and after;
from that call onward the News module does the download, signature check,
decryption and storage, and the stock applet renders the result. No article
bytes pass through homebrew, and no server change is needed — it runs
against the deployed v006 fixtures, with the existing patches and DNS.

Open: the filter argument is a NUL-terminated string whose accepted form is
undocumented. Three spellings (`topic_id='openpak_news'`, `openpak_news`,
`topic_id="openpak_news"`) are probed read-only through GetSubscriptionStatus
and only a filter naming our own topic is ever used to write — `"1"` would
subscribe the console to every topic. The per-candidate result codes from
the first hardware run are the finding, whatever they say.

If reception works, the refresh schedule stays the next unknown: an
immediate receive is not proof of unattended refresh.

**Corrected 2026-09-23:** v0.21 wrote status **1**, which 22.5.0 treats as
*unsubscribed* (0 forget, 1 unsubscribed, 2 subscribed, 3 auto). The
accepted spelling is the bare topic id. Probe v0.22 writes 2.

## Local sync fallback (2026-09-16)

If the firmware downloader cannot be driven, a second path reuses the
mechanism hardware already accepted in the v0.12 local test:
`newsPostLocalNews` takes a MessagePack article directly. Here homebrew
downloads and the firmware only stores and renders.

So the seed's decoded `article.msgpack` is now also served unwrapped:

- `tools/news-cdn/sign-seed.sh` copies each article payload into the signed
  tree as `<topic>/<news_id>/local.msgpack`. No key is involved.
- `/openpak/v1/local/index` lists one `<topic> <news_id> <bytes>` line per
  postable article, sorted, so two builds of a tree serve identical bytes.
- `/openpak/v1/local/<topic>/news/<news_id>` serves that payload verbatim.
- Probe **v0.20**, D-pad Up, downloads the index, downloads each article not
  in `sd:/switch/openpak-news-probe/posted.txt`, and posts it.

This is the fallback, not the goal: what it does **not** need is the
BCAT verification-key patch, the
edge-token patch, the DNS redirect, a container, an AES or signing key, an
edge token. It connects to the OpenPak address with the public OpenPak CA —
the transport v0.18 already proved from homebrew. What it does not provide:
background refresh. The player runs the NRO; that is the refresh.

Deploy: rebuild the seed, re-sign (the script now emits `local.msgpack`),
deploy as a new image + fixture directory and restart. cdn-v003 predates the
sync routes, so its index would be empty. Rollback is the previous image;
these routes are additive and change no existing response.

Unverified: whether the applet renders an article whose `topic_id` is an
OpenPak channel rather than a known Nintendo one. The v0.12 article that
rendered used `nx_news`; the seed articles carry their own channel id with
full topic name, image and description in the payload. If the cards do not
appear, that field is the first thing to test.

## Hardware check

With existing patches and OpenPak DNS, reopen News → Find Channels → OpenPak
News. Check for three cards, then open one and confirm body/images. Correlate
archive, summary and article requests with the UI result. No new NRO or reboot
is required for this server update. A visible channel card is not evidence of
background subscription delivery or persistence through reboot.

## Reception path in 22.5.0 (2026-09-23)

From the firmware audit (facts and addresses in [provenance](provenance.md)).
None of this is tested on hardware yet.

1. The topic is subscribed with status 2: Find Channels' follow, probe v0.22
   D-pad Down, or auto-subscription on title launch.
2. On a push, a country change or `RequestImmediateReception`, the bcat
   worker gets an edge token, then fetches
   `https://bcat-list-lp1.cdn.nintendo.net/api/nx/v1/list/<topic>?l=<lang>[&c[]=CC]`.
   The request carries `If-None-Match` or `If-Range`+`Range`, and a 304
   counts as success. There is no periodic polling.
3. The list is a signed/encrypted container like every other, whose
   plaintext is:

   ```json
   {
     "topic_id": "openpak_news",
     "service_status": "in_service",
     "na_required": false,
     "directories": [{"data_list": [{
       "version": {"format": 1, "semantics": 1},
       "news_id": 2609110005,
       "publishing_time": 1790150400,
       "deletion_priority": 100,
       "default_language": "en-GB",
       "languages": [{
         "language": "en-GB",
         "data_id": 123456789,
         "url": "https://bcat-topics-lp1.cdn.nintendo.net/openpak/v1/topics/openpak_news/news/2609110005/article",
         "size": 47945,
         "overwrite": false
       }]
     }]}]
   }
   ```

   Key order matters: `data_list` is rejected unless `topic_id`,
   `service_status` and `na_required` came first. `language` must lead
   each edition. `service_status: "expired"` retires the topic.
   `test_distribution: true` (optional, before `directories`) bypasses the
   history.
4. For each entry, the module downloads the selected edition's `url` (the
   signed article container, exactly `size` bytes). It skips news ids
   already in `history.bin` unless `overwrite` is set. It inserts the
   record with `opted_in=1, newly=1, read=0`, after purging expired rows.
5. qlaunch's main list shows `read=0 AND pickup_limit>now AND
   (expire_at=0 OR expire_at>=now) AND opted_in=1 AND 50<priority<1000`.
   Articles now carry priority 100.

What OpenPak builds and serves for this (all on the CDN's `:20040`, matched
by path whatever the Host, each with a strong ETag, 304 and Range support):

| Route | Fixture |
| --- | --- |
| `/api/nx/v1/list/<topic>` | `<topic>/list.bcat`, or `retired/<topic>/list.bcat` (expired) for a removed channel |
| `/openpak/v1/topics/<t>/news/<id>/article` | `<t>/<id>/article.bcat` (list `url`) |
| `/openpak/v1/topics/<t>/news/<id>/expired` | `retired/<t>/<id>/article.bcat`, the retired article's replacement, listed with `overwrite` |
| `/api/nx/v1/titles/<tid16>/topics` | `titles/<tid>.bcat`, else `titles/none.bcat` (an empty array) |
| `/api/nx/v2/latest_news/<n>` | the newest article's `summary.bcat` (qlaunch's request; its trigger is unknown) |

`bcat-list-lp1` resolves to nx-baas, which forwards `openpak_*` lists and
`titles/*/topics` here. The article URLs stay on `bcat-topics-lp1`, which
already reaches the CDN directly. The list and titles prefixes never fall
back to nx-baas, which would loop. The v1 `topics/<t>/archives` and
`topics/<t>/<id>/article` routes remain, but 22.5.0 builds neither.

Update and retirement: an article whose content changes gets
`overwrite: true` from then on. So does a retired article (listed via its
expired replacement, which the console then purges as expired) and an
unretired one. Republishing unchanged content at a new `--published` does
not set it. Whether an overwrite resets a card the player already read is
unknown.
