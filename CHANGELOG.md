# Changelog — news

Generated from git history on 2026-09-15. `git log` stays the source
of truth; this file is the readable summary.

## v0.2.2 — 2026-10-03

- Decoded copies for clients without the News key (an emulator answering the firmware's News downloads itself): the signers (`sign-seed.sh`, the admin publisher) leave `catalog.plain` and each live channel's `detail.plain`, `icon.plain`, `archives.plain` beside the containers, and `newscdn` serves them at `/openpak/v1/local/catalog` and `/openpak/v1/local/<topic>/{detail,icon,archives}`. Optional: a tree without them serves no such route. Containers and console routes are unchanged.

## v0.2.1 — 2026-10-03

- Articles without an essential pickup window no longer carry `essential_pickup`. Since v0.2.0 they carried `{pickup_limit: 0, priority_after}`, which qlaunch 22.5.0's handler (0x090867c0) rejects; its article parsers then stop before `subject`, so tiles and article pages showed images and no text. Found on the firmware HOME menu running in the emulator; the same code runs on a console. Every article is rebuilt (and marked overwrite) at the next publish.

## v0.2.0 — 2026-09-23

- Native News admin API (`newscdn -data`, `docs/admin-api.md`; not deployed):
  - channel and article CRUD over a writable `/data` source (the channels/ format), with image uploads for channel icons and article banners
  - `POST /publish` builds with `internal/newsseed`, signs and verifies every container in-process against the committed console key, and swaps the served tree atomically; `POST /rollback`, `GET /status`, last N builds kept
  - overwrite continuity across publishes and rollbacks (baseline = newest build's manifest)
  - pushes `{topic_id, revision}` to nx-baas for changed topics (failures logged, not fatal)
  - article `published_at` (stamped at first publish), `priority`, `pickup_days`, `essential_pickup`; channel icon upload
  - `cmd/newsseed` build logic moved to `internal/newsseed` (byte-identical output); the image is Alpine + cjpeg
- Native News, from the 22.5.0 firmware audit (not deployed, untested on hardware):
  - bcat-list per topic (`/api/nx/v1/list/<topic>`), signed like every other container, in the parser's key order; entries link each article container with its exact size
  - list `overwrite` on content change, retirement and unretirement; `service_status: "expired"` for removed channels
  - auto-subscription topic arrays (`/api/nx/v1/titles/<tid>/topics`) from channel `application_ids`; `/api/nx/v2/latest_news/<n>` answers with the newest summary
  - strong ETags, 304 and Range/If-Range on every News response; the list, titles and latest_news routes never fall back to nx-baas
  - priority 100 (qlaunch lists 50 < priority < 1000), nested `essential_pickup`, article `more` links, channel `shop_query`; `latest_news_urls` dropped
  - probe v0.22 subscribes with status 2 (1 means unsubscribed) using the bare topic id
  - docs: `provenance.md`, reception path, corrected freshness-gating and archive-route claims

- Native News, 2026-09-16 hardware run: seed expanded to 33 articles with real screenshots,
  plaintext article payloads and native v1 archive/article routes in `newsexperiment`,
  news-probe v0.21 (subscription and reception over IPC), `news-list-no-dauth` edge-token patch
- ci: tests on self-hosted runners, fixed-port integration tests serialized, registry auth
  scoped to release steps
- release: job-scoped registry auth file [fce777c]
- release: build on the self-hosted runner with podman; private-repo minutes are metered [c3c596f]
- Native News: containers, the CDN fixture, patch tooling and the hardware notes [b067f92]


## v0.1.0 — 2026-09-07

- Ship a container image on tag: rootless-ready Dockerfile + ghcr release workflow [4ba1611]
- news: apply NB-00 review — console surfaces NO-GO, emulator delivery is the Switch channel [c09b998]
- news: platform-neutral news service + Switch BCAT renderer (PRD NB-00..NB-08) [df0ae6a]

