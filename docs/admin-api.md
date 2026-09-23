# News admin API (contract)

The internal HTTP API the website admin dashboard uses to author native
Switch News channels and articles, publish them, and roll back. It is served
by the running `newscdn` (container `openpak-news-experiment`) on a second,
**plain-HTTP** listener that is reachable only on the internal `openpak`
network. The TLS console listener (`:20040`) is unchanged.

Signing happens on the server: `POST /publish` rebuilds the seed from the
source of truth in `/data`, signs every container in-process, verifies each
one against the committed OpenPak public key, writes a new versioned fixture
tree and switches the console routes to it atomically, without a restart.

## Connection

| Setting | Value |
| --- | --- |
| Base URL (website `NEWS_ADMIN_URL`) | `http://openpak-news-experiment:20043` |
| Auth header | `X-Internal-Key: <NEWS_ADMIN_KEY>` on every request |
| Prefix | every path below starts with `/admin/v1` |
| Bodies | JSON (`Content-Type: application/json`), except image uploads |

- The server compares the key in constant time. A missing or wrong key
  answers `401`. If the server was started without `NEWS_ADMIN_KEY`, **every**
  request answers `503` (the API is disabled, not open).
- Nothing in any response contains key material or key paths.
- Every mutation edits the source only. Nothing reaches consoles until
  `POST /admin/v1/publish`. `GET /admin/v1/status` reports `dirty: true` while
  the source differs from the served build.
- Requests are serialised: a publish waits for an in-flight edit and vice versa.

### Errors

Every non-2xx response is `{"error": "<human-readable message>"}`.

| Status | Meaning |
| --- | --- |
| 400 | Malformed JSON, unknown field, or a field fails validation (the message names the field) |
| 401 | Missing or wrong `X-Internal-Key` |
| 404 | Unknown channel, article, image or route |
| 409 | Conflict: topic id or news id already exists, retired-state conflict, nothing to roll back to |
| 413 | Body too large (JSON 256 KiB, image 16 MiB) |
| 415 | Image upload that is not PNG or JPEG |
| 422 | `publish`: the source does not build (the message says why; nothing was switched) |
| 500 | Signing, verification or I/O failure (nothing was switched) |
| 503 | API disabled (`NEWS_ADMIN_KEY` unset), or `publish` without signing keys configured |

## Channels

### Channel object

```json
{
  "topic_id": "openpak_news",
  "name": "OpenPak News",
  "publisher": "OpenPak",
  "description": "Announcements and milestones from the OpenPak network.",
  "important": false,
  "icon": {"motif": "mark", "hue": 0},
  "has_icon_image": false,
  "shop_query": "",
  "application_ids": ["0100abcd12340000"],
  "test_distribution": false,
  "retired": false,
  "article_count": 8,
  "active_article_count": 8
}
```

| Field | Rules |
| --- | --- |
| `topic_id` | `^openpak_[a-z0-9_]{1,23}$` (at most 31 bytes, the firmware limit). Set on create, **immutable** afterwards: it is in every console request path |
| `name`, `publisher` | Required, 1..100 bytes |
| `description` | Required, 1..120 bytes |
| `important` | Boolean, default `false` |
| `icon.motif` | One of `mark`, `mark-pad`, `mark-gear`, `mark-question` (see `GET /motifs`). Default `mark`. Used when no icon image is uploaded |
| `icon.hue` | Integer, accepted and stored, currently unused by the renderer |
| `has_icon_image` | Read-only. `true` when an uploaded icon replaces the motif |
| `shop_query` | Optional raw eShop query for the channel page's shop button, at most 1024 bytes. Empty = no button |
| `application_ids` | Optional title ids, 16 lowercase hex digits each, at most 64. Launching one of those titles auto-subscribes the console to the channel |
| `test_distribution` | Boolean. Makes consoles ignore their received-id history for this channel. **Test channels only** |
| `retired` | Read-only here; change it with `PATCH` (below) |
| `article_count`, `active_article_count` | Read-only. All articles / articles not retired |

### Endpoints

| Method and path | Body | Answer |
| --- | --- | --- |
| `GET /admin/v1/channels` | – | `200 {"channels": [Channel…]}` (lineup order, then retired channels) |
| `POST /admin/v1/channels` | Channel fields (`topic_id`, `name`, `publisher`, `description` required) | `201 Channel`. `409` if the topic id exists (live or retired) |
| `GET /admin/v1/channels/{topic}` | – | `200 Channel` |
| `PATCH /admin/v1/channels/{topic}` | Any subset of the writable fields, plus `retired` and `retire_reason` | `200 Channel` |
| `PUT /admin/v1/channels/{topic}/icon` | Raw PNG or JPEG bytes, `Content-Type: image/png` or `image/jpeg` | `200 Channel` (`has_icon_image: true`) |
| `GET /admin/v1/channels/{topic}/icon` | – | `200` the uploaded image as stored, `404` if none |
| `DELETE /admin/v1/channels/{topic}/icon` | – | `200 Channel` (back to the motif) |

`PATCH` is a JSON merge: absent keys keep their value, `null` resets a key to
its default (empty `shop_query`, no `application_ids`, …). `topic_id` cannot be
changed (`400`). Unknown keys are rejected (`400`).

The icon is rendered at publish time to the 70×70 baseline JPEG the console
accepts, center-cropped. Images: PNG or JPEG, at most 16 MiB and 8192×8192.

**Retiring a channel** — `PATCH {"retired": true, "retire_reason": "…"}`
(`retire_reason` required, at most 200 bytes). On the next publish the
channel leaves the catalog, every article gets its expired replacement
(listed with `overwrite`, so consoles replace their copies), and the
channel's list answers `service_status: "expired"`, which retires the topic
on subscribed consoles. Its articles become read-only while it is retired.
`PATCH {"retired": false}` restores the channel to the end of the lineup;
its articles **stay retired** until each is un-retired individually. A
retired channel can be edited (name, icon…) only after it is restored.

## Articles

### Article object

```json
{
  "topic_id": "openpak_news",
  "news_id": 2609230001,
  "title": "Welcome to the OpenPak channel lineup",
  "body": "One paragraph of text.",
  "footer": "",
  "language": "en-GB",
  "supported_languages": ["en-GB", "en-US"],
  "lifetime_days": 14,
  "published_at": "2026-09-23T12:00:00Z",
  "expire_at": "2026-10-07T12:00:00Z",
  "pickup_days": null,
  "priority": 100,
  "essential_pickup": null,
  "art": {"motif": "burst", "hue": 0},
  "has_image": false,
  "more": {"type": "browser", "url": "https://openpak.example/guide"},
  "retired": false,
  "retire_reason": "",
  "ever_published": true
}
```

| Field | Rules |
| --- | --- |
| `news_id` | Optional on create; the server allocates `YYMMDD` (UTC today) + a 4-digit sequence. If given: 1..4294967295 and unused by **any** article of any channel, including retired ones. Immutable. Never reused |
| `title` | Required, 1..100 bytes |
| `body` | Required, 1..1200 bytes, **one paragraph** (no `\n` / `\r`: newlines are unverified in the native reader) |
| `footer` | Optional, at most 80 bytes. Empty = `"Published by OpenPak."` |
| `language` | Required, `ll-CC` (for example `en-GB`) |
| `supported_languages` | Required, `ll-CC` each, must include `language` |
| `lifetime_days` | Required, 1..30. `expire_at = published_at + lifetime_days` |
| `published_at` | Optional RFC 3339 UTC (`Z`). `null` = stamped with the publish time the first time this article is published, then kept. Setting it moves `expire_at` and pickup windows; it is **not** a content change (see Overwrite) |
| `expire_at` | Read-only, derived. `null` until `published_at` is known |
| `pickup_days` | Optional, 0..`lifetime_days`. How long the unread card stays in the main News list (`pickup_limit = published_at + pickup_days`). `null` = until expiry |
| `priority` | Optional, default 100. Must be 51..999 (the main list band) or 1000..1998 (the second band). Values ≤ 50 are stored by consoles but never listed, so they are rejected |
| `essential_pickup` | Optional `{"pickup_days": 1..lifetime_days, "priority_after": 51..1998}`; `priority_after` defaults to `priority`. `null` = none (sent as `pickup_limit 0`). Semantics are inferred from the firmware, unverified on hardware |
| `art.motif` | Required on create. A scene motif from `GET /motifs`. Used when no image is uploaded |
| `art.hue` | Integer, stored, currently unused |
| `has_image` | Read-only. `true` when an uploaded image replaces the generated banner (card 358×201 and hero 800×364, center-cropped) |
| `more` | Optional link button, `null` = none. `{"type": "game", "application_ids": [1..5 title ids]}`, `{"type": "shop", "query": "…"}` (or `application_ids`), `{"type": "browser", "url": "https://…"}` (at most 256 bytes). Only these three types |
| `retired`, `retire_reason` | Read-only here; change with `PATCH` (below) |
| `ever_published` | Read-only. `true` once any publish has shipped this news id to consoles |

Title ids are 16 lowercase hex digits (`0100abcd12340000`).

### Endpoints

| Method and path | Body | Answer |
| --- | --- | --- |
| `GET /admin/v1/channels/{topic}/articles` | – | `200 {"articles": [Article…]}`, newest `news_id` first |
| `POST /admin/v1/channels/{topic}/articles` | Article fields | `201 Article`. `409` for a used `news_id` or a retired channel |
| `GET /admin/v1/channels/{topic}/articles/{id}` | – | `200 Article` |
| `PATCH /admin/v1/channels/{topic}/articles/{id}` | Any subset of writable fields, plus `retired` and `retire_reason` | `200 Article` |
| `DELETE /admin/v1/channels/{topic}/articles/{id}` | – | `200 {"deleted": true}` if it was never published; `200 {"retired": true, "article": Article}` if it was (see below) |
| `PUT /admin/v1/channels/{topic}/articles/{id}/image` | Raw PNG or JPEG | `200 Article` (`has_image: true`) |
| `GET /admin/v1/channels/{topic}/articles/{id}/image` | – | `200` the uploaded image as stored, `404` if none |
| `DELETE /admin/v1/channels/{topic}/articles/{id}/image` | – | `200 Article` (back to the motif) |

`PATCH` is a JSON merge, as for channels. `news_id` and `topic_id` cannot be
changed.

**Retiring an article** — `PATCH {"retired": true, "retire_reason": "…"}`
(reason required). The next publish removes it from the archive and lists
its expired replacement with `overwrite`, so consoles holding it replace it
and purge it at their own housekeeping (no instant delete is guaranteed).
`PATCH {"retired": false}` restores it. A retired article cannot be edited
until it is restored.

**Deleting** — an article that never shipped (`ever_published: false`) is
deleted outright. One that did ship is **retired instead** (reason
`"deleted from the admin dashboard"`), because consoles may hold it and its
id must never be reused. The response says which happened.

### Overwrite

Consoles skip a news id they have already received unless its list entry
sets `overwrite`. The server sets it automatically, and keeps it set, when
an article's content changes after it was published: title, body, footer,
languages, lifetime, pickup, priority, essential pickup, art or image,
`more`, or its channel's name / publisher / description / icon. Moving
`published_at` alone is not a content change. Retiring and un-retiring set it
too. Publish always compares against the newest build ever made (not the one
currently served), so a rollback followed by a new publish still sets it.

## Publishing

### `POST /admin/v1/publish`

Body optional: `{"note": "free text, at most 200 bytes"}`.

1. Stamps `published_at` on never-published articles (persisted only if the
   publish succeeds).
2. Builds the seed from `/data/channels` with the newest build's
   `manifest.json` as the previous state (overwrite continuity).
3. Signs every payload (RSA-2048 PKCS#1 v1.5 / SHA-256 over the plaintext,
   AES-CTR, fresh IV, secret index 0) and verifies every container against
   the committed OpenPak public key: size = 288 + payload, decrypts to the
   exact payload, signature valid.
4. Writes `/data/builds/<build>/signed`, loads it as the console route table,
   records the state, then switches the served tree atomically. In-flight
   console requests finish on the old tree.
5. Pushes `{topic_id, revision}` to nx-baas for every topic whose bcat-list
   changed (asynchronously; a push failure is logged and shown in `status`,
   it does not fail the publish).

`200`:

```json
{
  "build": 7,
  "revision": 12,
  "published_at": "2026-09-23T12:00:00Z",
  "containers": 117,
  "channels": 4,
  "articles": 33,
  "changed_topics": ["openpak_news"],
  "push": "queued",
  "note": "fix typo"
}
```

`push` is `"queued"`, `"disabled"` (no `NX_BAAS_PUSH_URL`) or `"none"` (no
topic changed). A failed publish (`422` / `500` / `503`) switches nothing,
keeps the source as it was and records `last_error`.

### `POST /admin/v1/rollback`

Body optional: `{"build": N}`. Without it, switches back to the build that
was served before the current one; with it, to any kept build. `200` with
the same shape as publish (`build` is the build now served, `revision` a new
revision, `changed_topics` compared with the build that was served). `409` if
there is nothing to roll back to or the build is not kept.

Rollback changes what consoles are served, **not** the source: the edits
stay in `/data/channels` (`status.dirty` becomes `true`) and the next publish
ships them again. To undo an edit, edit it back and publish.

### `GET /admin/v1/status`

```json
{
  "serving": {"build": 7, "since": "2026-09-23T12:00:05Z"},
  "revision": 12,
  "dirty": false,
  "signing_configured": true,
  "push_configured": true,
  "last_publish": {"build": 7, "at": "2026-09-23T12:00:05Z", "changed_topics": ["openpak_news"], "containers": 117, "note": ""},
  "last_error": null,
  "pushes": [{"topic_id": "openpak_news", "revision": 12, "at": "2026-09-23T12:00:06Z", "ok": true, "status": 200, "error": ""}],
  "builds": [{"build": 7, "created_at": "2026-09-23T12:00:05Z", "containers": 117, "channels": 4, "articles": 33, "note": ""}]
}
```

- `serving.build` is `null` while the service still serves the static fixture
  tree it was started with (before the first publish).
- `last_error` is `null` or `{"at", "op": "publish"|"rollback"|"push", "error"}`.
- `pushes` lists the results of the most recent push round.
- `builds` lists the kept builds, newest first (the server keeps the last N,
  default 10, and never deletes the served one).

### `GET /admin/v1/motifs`

`200 {"icon": ["mark", "mark-gear", "mark-pad", "mark-question"], "scene": ["beacon", "block", …]}`
— the valid `icon.motif` and `art.motif` values.

## Limits and non-goals

- One language edition per article (`language` + `supported_languages`), as
  in the file-based seed.
- Channels cannot be deleted, only retired; topic ids and news ids are never
  reused.
- Images are stored as uploaded and re-encoded at publish time; the
  third-party-rights rule of `channels/README.md` applies to uploads.
- This API never returns or accepts key material.
