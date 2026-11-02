# Notes: news

> **Current Switch client (2026-10-13): Harbor**, at
> `~/REPOS/Openpak/emulators/harbor`. Harbor is an independent
> OpenPak project based on Eden. Citron, Ryujinx (desktop and Android), and
> the former OpenPak Eden checkouts have been removed. References to those
> forks below record historical evidence only: do not run their commands,
> reinstall them, or select them for current work. Use Harbor's README and
> current handoff for builds and testing; old results do not establish Harbor parity.

**Game BCAT:** one shared dataset service serves Harbor's
public JSON/file API through the website and native signed game BCAT through
nx-baas. More titles use the same
per-title contract. See the
[shared native service](native-game-bcat.md) and
[dataset/deployment runbook](emulator-bcat-delivery.md).

**Server-side authoring:** `newscdn -data`
runs an internal admin API ([contract](admin-api.md)) that the website
dashboard uses to create and edit channels and articles and to publish them.
Publishing signs in-process on the server and switches the console tree
atomically, without a restart. See [Native News admin API](protocol.md#native-news-admin-api-newscdn--data).

Start native Switch work with the [creation guide](native-news-authoring-guide.md),
[editable sample](../examples/native-news/README.md), and [channel lineup
seed](../channels/README.md).

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
