# Channel archive and article delivery experiment

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
The independent `latest_news_urls` field in channel detail stays empty because
its subscription-delivery schema has not been established.

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

## Hardware check

With existing patches and OpenPak DNS, reopen News → Find Channels → OpenPak
News. Check for three cards, then open one and confirm body/images. Correlate
archive, summary and article requests with the UI result. No new NRO or reboot
is required for this server update. A visible channel card is not evidence of
background subscription delivery or persistence through reboot.
