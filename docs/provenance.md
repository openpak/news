# Provenance — native News facts

Where each non-obvious fact behind the native News routes and payloads came
from ([clean-room policy](https://docs.openpak.org/contributors/clean-room/)). Firmware facts come from
static analysis of HOS 22.5.0 by the owner, recorded in
internal research notes (the audit
workspace is `~/openpak-firmware-audit-22.5.0`). No decompiled code is
copied into this repository: only names, values, orders and flows.

Addresses: bcat `FUN_`/0x values are Ghidra addresses (base 0x100000);
qlaunch values are ELF vaddrs unless noted. **V** = seen in the binary,
**I** = inferred, **H** = observed on hardware, **U** = unexplained.

## Subscription and reception

| Fact | Source |
| --- | --- |
| SetSubscriptionStatus (40100) values: 0 forget, 1 unsubscribed, 2 subscribed, 3 auto | V, bcat news dispatch `FUN_001f5630`; gap report P0 #1 |
| The bcat worker fetches only topics in status 2 and refuses to store records for other topics (0xd47d) | V, gap report P0 #1 |
| The topic argument is the bare topic id; `topic_id='…'` spellings fail with 0x47d | H, probe v0.21, 2026-09-16 |
| Topic id `[A-Za-z0-9_]{1,31}`; at most 256 subscriptions | V, gap report "News — detail" |
| Reception flow: edge token (0x117284), then GET `https://bcat-list-%.cdn.nintendo.net/api/nx/v1/list/<topic>?l=<lang>[&c[]=CC]`, decrypted with the News key and stored as `news-dl:/list.msgpack` (≤ 0xe10000 bytes). Then, for each entry, GET the language's `url` (https only) into `news-dl:/data.msgpack`, check history, insert with `opted_in=1, newly=1, read=0` | V, bcat strings `https://%s/api/nx/v1/list/%s%s%s` (0x30f72c), `bcat-list-%.cdn.nintendo.net` (0x30eb1b); gap report |
| No periodic polling. Refresh comes from an NPNS push `{"type":"news","topic_id","revision","wait_range"}` (random delay within `wait_range`, default 3600 s), a change in the accounts' countries, or RequestImmediateReception (30300) | V (parse), I (NPNS topic naming); gap report |
| Retry backoff 5/10/15/30/60 minutes | V, gap report |
| Request headers: `X-Nintendo-DenebEdgeToken`, `X-Nintendo-Serial-Number`, `If-None-Match` or `If-Range`+`Range`; a 304 counts as success | V, bcat strings 0x3110af, 0x309640, 0x308c8b; gap report |
| No freshness gate at store time. Expired rows are purged before each insert; the limit is 300 records | V, record parser `FUN_00208450`, `0<expire_at AND expire_at<%lld` (0x31048a) |
| The received-id history (`news-sys:/history.bin`) skips a news id already seen unless the list language sets `overwrite` or the list sets `test_distribution` | V, gap report P1 |
| Correction: the 2026-09-16 "storage rejected / freshness gating" theory was wrong. The cause was status 1. The 18:10 downloads were qlaunch channel-page requests, not the module's | V, gap report "Docs the binary contradicts" |
| `/api/nx/v1/topics/<t>/archives` and `/api/nx/v1/topics/<t>/<id>/article` are built by neither qlaunch nor bcat in 22.5.0 | V, gap report. The 2026-09-16 log lines that looked like them are **U** |

## bcat-list response (parser `FUN_00215590`, entries `FUN_00215880`, languages `FUN_00215d10`)

| Fact | Source |
| --- | --- |
| Top-level keys read: `$.topic_id` (≤ 0x20 bytes), `$.service_status`, `$.na_required`, optional `$.test_distribution`, then `$.directories[0].data_list[]` | V |
| `data_list` is accepted only after `topic_id`, `service_status` and `na_required` have all been seen, so key order matters; a missing key gives 0x67d | V |
| `service_status` is `in_service` or `expired`; any other string gives 0x67d; `expired` gives 0x3c7d and retires the topic | V (strings), gap report (result codes) |
| `na_required` true means a Nintendo Account link is required (0x3e7d) | V, gap report |
| Directory object: only `data_list` of `directories[0]` is read. **U**: whether Nintendo's directories carry other fields (a name, a mode) was not recovered. OpenPak emits the minimal `{data_list}` | V (paths), U (other fields) |
| Entry keys: `version.format` (=1), `version.semantics` (≤ `news!system_version`, else 0x407d), `news_id`, `publishing_time`, `deletion_priority`, `default_language` (≤ 8 bytes), `languages[]`, optional `conditions.serial[*]` (≤ 10) | V |
| Language keys: `language`, `data_id`, `url` (≤ 0x100 bytes), `size`, `overwrite`. `data_id`, `url` and `size` are all present or all absent. `data_id`, `size` and `news_id` are read as 64-bit integers | V |
| A language edition is selected by `language`: an exact match with the console wins, a `default_language` match is the fallback, and only the selected edition's other keys are read. So `language` comes first, and `default_language` must precede `languages` | V |
| `size` must equal the downloaded bytes | I, from the gap report schema. OpenPak serves the signed container, so size = 0x120 header + payload (`internal/bcatcontainer`) |
| **U**: what `data_id` identifies. OpenPak derives it from a hash of the article content, so it changes exactly when the content does | U |
| **U**: whether an overwrite re-insert resets `read`/`newly` on a card the player already read | U |
| **U**: whether the data URL download requires a signed container. OpenPak serves the same signed article container as everywhere else | I |

## Record (`FUN_00208450`) and main list (qlaunch 0xb7d6b0)

| Fact | Source |
| --- | --- |
| Required: `news_id`, `topic_id`, `published_at`, `priority`, `deletion_priority` | V |
| Optional: `expire_at`, `pickup_limit`, `essential_pickup.pickup_limit`, `essential_pickup.priority_after`, `application_ids[*]` (≤ 5), `age_limit`, `age_limits[*].ratings[*].age`, `decoration.type`, `language` | V, bcat strings 0x30ce68, 0x3112a0, 0x30ce53 |
| The flat `essential_pickup_limit` is not read | V |
| **U**: what `priority_after` means (I: the priority once the essential-pickup window ends). OpenPak sends `pickup_limit` 0 and `priority_after` 100 | U |
| Main list query: `read=0 AND pickup_limit>now AND (expire_at=0 OR expire_at>=now) AND opted_in=1 AND priority>50 AND priority<1000`, a `read!=0` variant, and a `1000≤priority<1999` band. Sorted by `reputation DESC, published_at DESC` | V |
| So priority 50 (sent until 2026-09-23) stored the record but never listed it. OpenPak now sends 100 | V |

## Channel detail, article UI, auto-subscribe, latest news

| Fact | Source |
| --- | --- |
| `latest_news_urls` is parsed by nothing in 22.5.0; the key is no longer emitted | V, gap report P2 |
| `shop_query` exists in qlaunch (channel eShop button) and is passed raw to the shop applet | V (string, qlaunch strings offset 12435269), I (placement in detail) |
| Article `more` keys: `type` (`shop`, `browser`, `game`, `system_applet`, `nso`), `query`, `url`, `relative_url`, `application_ids`, `application_arg`, `allow_domains` | V (keys), I (semantics); gap report |
| **U**: the msgpack type of `more.application_ids`. OpenPak emits integers, like the record's `application_ids`. Which keys each `type` reads is also **U**; OpenPak emits only the keys the author set | U |
| Auto-subscribe: `https://%s/api/%s/v1/titles/%016llx/topics%s%s` on `bcat-topics-%.cdn.nintendo.net` expects a signed msgpack **array of topic-id strings** | V, bcat string 0x30d844 (FUN_00213d60); gap report P1 |
| **U**: the array limit. OpenPak caps it at 64 | U |
| Who calls RequestAutoSubscription (40101) | U, gap report "Needs hardware" |
| `latest_news`: qlaunch builds `%s/api/nx/v2/latest_news/%s?l=%s` on `https://bcat-topics-%.cdn.nintendo.net` | V, qlaunch strings offset 12600107 |
| **U**: what triggers the `latest_news` request and what its body is. OpenPak answers with the newest article's summary container | U |

## Earlier hardware facts still relied on

| Fact | Source |
| --- | --- |
| Container: 0x120 header, RSA-2048 PKCS#1 v1.5 SHA-256 over plaintext, AES-CTR, IV at 0x10 | H, probe v0.19 catalog decode, 2026-09-09; `docs/native-news-container.md` |
| online_archives entry and summary schema | qlaunch trace, `docs/native-news-article-delivery.md` |
| NORMAL article layout (subject, body map, images) | H, probe v0.12 local post |
