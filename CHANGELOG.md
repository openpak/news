# Changelog — news

Generated from git history on 2026-09-15. `git log` stays the source
of truth; this file is the readable summary.

## Unreleased

- Native News, from the 22.5.0 firmware audit (not deployed, untested on hardware):
  - bcat-list per topic (`/api/nx/v1/list/<topic>`), signed like every other container, in the parser's key order; entries link each article container with its exact size
  - list `overwrite` on content change, retirement and unretirement; `service_status: "expired"` for removed channels
  - auto-subscription topic arrays (`/api/nx/v1/titles/<tid>/topics`) from channel `application_ids`; `/api/nx/v2/latest_news/<n>` answers with the newest summary
  - strong ETags, 304 and Range/If-Range on every News response; the list, titles and latest_news routes never fall back to nx-baas
  - priority 100 (qlaunch lists 50 < priority < 1000), nested `essential_pickup`, article `more` links, channel `shop_query`; `latest_news_urls` dropped
  - probe v0.22 subscribes with status 2 (1 means unsubscribed) using the bare topic id
  - docs: `provenance.md`, reception path, corrected freshness-gating and archive-route claims

- release: job-scoped registry auth file [fce777c]
- release: build on the self-hosted runner with podman; private-repo minutes are metered [c3c596f]
- Native News: containers, the CDN fixture, patch tooling and the hardware notes [b067f92]


## v0.1.0 — 2026-09-07

- Ship a container image on tag: rootless-ready Dockerfile + ghcr release workflow [4ba1611]
- news: apply NB-00 review — console surfaces NO-GO, emulator delivery is the Switch channel [c09b998]
- news: platform-neutral news service + Switch BCAT renderer (PRD NB-00..NB-08) [df0ae6a]

