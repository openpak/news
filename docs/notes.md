# Notes: news

> **Current Switch client (2026-10-13): Harbor**, at
> `~/REPOS/Openpak/emulators/harbor`. Harbor is an independent
> OpenPak project based on Eden. Citron, Ryujinx (desktop and Android), and
> the former OpenPak Eden checkouts have been removed. References to those
> forks below record historical evidence only: do not run their commands,
> reinstall them, or select them for current work. Use Harbor's README and
> current handoff for builds and testing; old results do not establish Harbor parity.

**Game BCAT (2026-10-12):** one shared dataset service now serves Harbor's
public JSON/file API through the website and native signed game BCAT through
nx-baas. Splatoon 2 is the first published dataset; more titles use the same
per-title contract. Harbor lobby entry and Regular Battle search were tested;
native 23.0.1 console acceptance still needs a hardware test. See the
[shared native service](native-game-bcat.md) and
[dataset/deployment runbook](emulator-bcat-delivery.md).

**Current deployment (2026-09-10):** owner-authorized v006/cdn-v003 now serves four channels, populated archives, twelve summaries and linked full articles. Host and public HTTPS verification pass; native rendering and automatic subscription delivery remain pending. See [current delivery, evidence and rollback](native-news-article-delivery.md). Earlier empty-archive/preparation statements below are historical.

**Server-side authoring (2026-09-23, committed, not deployed):** `newscdn -data`
runs an internal admin API ([contract](admin-api.md)) that the website
dashboard uses to create and edit channels and articles and to publish them.
Publishing signs in-process on the server and switches the console tree
atomically, without a restart. See [Native News admin API](protocol.md#native-news-admin-api-newscdn--data).

Start native Switch work with the [creation guide](native-news-authoring-guide.md),
[editable sample](../examples/native-news/README.md), [channel lineup
seed](../channels/README.md), and [agent handoff](native-news-agent-handoff.md).

One platform-neutral news service for the whole OpenPak network, plus
emulator delivery of per-title Switch BCAT data (PRD:
`prds/news-bcat-prd.md` as corrected by `docs/spike-nb00-review.md`). An
article is written once — targeted by **platforms, titles, regions,
languages** (empty = all) — and delivered through every channel a player
has: the website, companion apps and title servers via the read API.
Per-title BCAT datasets (schedules, seasonal content) are Switch-only by
nature and are delivered to emulators and to the LayeredFS placement flow.

License: AGPL-3.0-only (`LICENSE`, verbatim from gnu.org). Go 1.26,
module `github.com/openpak/news`.

## Layout (PRD §8)

```
cmd/news               the binary: authoring gRPC, read API, renderer, metrics
cmd/newsctl            operator CLI (stopgap authoring + acceptance journeys)
internal/content       model, audience targeting, read API (NB-03, NB-07)
internal/store         PostgreSQL implementation (pgx), embedded migrations,
                       Postgres-backed integration tests (NEWS_TEST_DATABASE)
internal/objects       content-addressed object store (images, dataset files)
internal/grpcapi       authoring gRPC (NB-2, NB-4), operator-key gated
internal/metrics       metrics/pprof listener (ports.md +2)
render/                renderer contract (NB-08): audience in, channel format out
render/switchbcat      Switch channel: emulator delivery of BCAT datasets
                       (NB-05) + delivery-manifest renderer; emulator-only
proto/, gen/           news.v1 authoring gRPC
docs/spike-nb00.md     NB-00 finding (corrected) — console surfaces NO-GO
docs/spike-nb00-review.md  owner review with the citations (authoritative)
```

Renderers never hold content of their own: `render/switchbcat` reads the
platform-neutral `content.Reader` only. A future SpotPass renderer
(Wii U/3DS, with `nn-boss`) implements the same `render.Renderer`
interface without touching content code.

## Status (PRD §10, as corrected)

- N0 spike finding written and **corrected** — console surfaces NO-GO
  (owner review accepted as the record).
- N1' emulator delivery end to end on Citron (article + dataset) —
  tooling ready; hardware gate owner-run.
- N2' LayeredFS placement contract for the setup homebrew — contract
  defined above; implementation lands with the homebrew.
- N3 authoring pages (website agent), read API + emulator delivery — in.
- N4 SpotPass renderer — unchanged (Wii U/3DS, when the first title lands).
