# news

Start native Switch work with the [creation guide](docs/native-news-authoring-guide.md),
[editable sample](examples/native-news/README.md), [channel lineup
seed](channels/README.md), and [agent handoff](docs/native-news-agent-handoff.md).

**Patched-console experiment (2026-09-09):** the owner has authorized and
installed a build-specific OpenPak BCAT verification-key patch. An isolated
remote CDN now serves original signed/encrypted catalog fixtures for testing
with probe v0.19 ZL. After a separate News edge-token omission patch and reboot,
native retrieval succeeds and reports 155 decoded catalog bytes. Exported
plaintext comparison, Find Channels parsing and subscriptions remain pending.
The UI has requested OpenPak catalog/icon/detail. Server v004 also serves an
empty online_archives container; the channel-page retest is pending.
See [experiment and rollback](docs/native-news-container.md).
The earlier server-only NO-GO below still applies to unpatched consoles.

One platform-neutral news service for the whole OpenPak network, plus
emulator delivery of per-title Switch BCAT data (PRD:
`prds/news-bcat-prd.md` as corrected by `docs/spike-nb00-review.md`). An
article is written once — targeted by **platforms, titles, regions,
languages** (empty = all) — and delivered through every channel a player
has: the website, companion apps and title servers via the read API.
Per-title BCAT datasets (schedules, seasonal content) are Switch-only by
nature and are delivered to emulators and to the LayeredFS placement flow.

**Console finding (NB-00, see `docs/spike-nb00.md` +
`docs/spike-nb00-review.md`):** the Switch's topics/list/data payloads are
signed, optionally encrypted **bcat content containers** — RSA-2048 with a
Nintendo key held in the sysmodule. Without Nintendo's private key (and
refusing console-side patches), console News and console BCAT are **NO-GO**.
At the time of that server-only spike, this repo served no bcat-hostname console routes; contract
request NB-1 is withdrawn. What reaches consoles is dataset placement on
the SD by the setup homebrew (LayeredFS), driven by this repo's delivery
contract.

License: AGPL-3.0-only (`LICENSE`, verbatim from gnu.org). Go 1.26,
module `github.com/openpak/news`.

**Native News follow-up (2026-09-07):** a separate local IPC route,
`newsPostLocalNews`, is now under investigation for a foreground homebrew
helper. The later v0.12 test demonstrated local article rendering; subscribable
channels and automatic delivery remain unverified. See
[the evidence and hardware experiment](docs/native-news-reopen.md).

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

## Endpoints

Authoring gRPC (operators only, `authorization: Bearer $NEWS_OPERATOR_KEY`
or `x-operator-key`): `news.v1.News` — `CreateArticle`, `UpdateArticle`,
`PublishArticle`, `UnpublishArticle`, `GetArticle`, `ListArticles`,
`UploadImage`, `RenderPreview`, `CreateDataset`, `PutDatasetFile`,
`SealDataset`, `ListDatasets` (`proto/news/v1/news.proto`).

Read API (public, website / apps / title servers, NB-07):

- `GET /v1/articles?platform=switch&title=<id>&region=FR&language=en-US`
  — JSON with `ETag` / `Last-Modified` / 304s.

Emulator delivery (NB-05, the Switch channel's live surface; mounted by
the adapter on its emulator API, ports.md 20070, or served standalone):

- `GET /api/emulator/v1/bcat/titles` — titles with served datasets
- `GET /api/emulator/v1/bcat/titles/<title>` — delivery manifest
  (paths, sizes, sha256, validity window, absolute file URLs)
- `GET /api/emulator/v1/bcat/titles/<title>/files/<path>` — dataset file
- `GET /images/<key>` — article images referenced by the read API

All responses carry `ETag`, `Last-Modified`, `Cache-Control` and answer
`If-None-Match` / `If-Modified-Since` (NB-06).

Ports follow `../ports.md` block 20040: HTTP `:20040`, gRPC `:20041`,
metrics/pprof `:20042` (`NEWS_HTTP_ADDR`, `NEWS_GRPC_ADDR`,
`NEWS_METRICS_ADDR`).

## LayeredFS placement contract (N2')

The setup homebrew is the console-side consumer. For each title id it
fetches the delivery manifest and places each file, by its `path` field,
into the title's BCAT directory on the SD:

- `path` is relative, slash-separated, no `..` (validated server-side);
- file integrity is the manifest `sha256` (`X-Content-SHA256` on fetch);
- a dataset is superseded by a newer sealed one whose `valid_from` is
  later; `valid_until` past its end means drop the files;
- placement is outside the bcat delivery cache, so no container signing
  is involved.

## Run

```sh
cp example.env .env  # fill in; never commit real values
createdb news        # or: docker exec openpak-pg-live psql -U postgres -c '...'
go run ./cmd/news
```

Migrations apply on boot (`internal/store/migrations`). Objects land in
`NEWS_OBJECTS_DIR` as content-addressed files; an S3-compatible store
(ports.md 23010) can replace the filesystem backend behind
`objects.Store` without touching callers.

## Authoring without the website

`cmd/newsctl` is the operator CLI (the website's role-gated pages consume
the same gRPC, NB-2):

```sh
newsctl -addr :20041 -key "$NEWS_OPERATOR_KEY" article create -slug openpak-live \
  -platforms all -locale "en-US=OpenPak is live" -body-file body.txt
newsctl -key ... article publish -id <id> -expires 2027-01-01T00:00:00Z
newsctl -key ... image upload -id <id> -file art.jpg
newsctl -key ... dataset create -title 0100ed9024eb8000 -until 2027-01-01T00:00:00Z
newsctl -key ... dataset put -dataset <id> -path event/season.bin -file season.bin
newsctl -key ... dataset seal -dataset <id>
```

### N1' journey — emulator delivery end to end

```sh
newsctl ... dataset create -title 0100ed9024eb8000 -until 2027-01-01T00:00:00Z
newsctl ... dataset put -dataset <id> -path event/season.bin -file season.bin
newsctl ... dataset seal -dataset <id>
curl localhost:20040/api/emulator/v1/bcat/titles
curl localhost:20040/api/emulator/v1/bcat/titles/0100ed9024eb8000
curl -o season.bin localhost:20040/api/emulator/v1/bcat/titles/0100ed9024eb8000/files/event/season.bin
```

Gate: Citron, pointed at the emulator API, consumes the dataset (article
+ data). Emulators take the BCAT passphrase from the game and verify no
container signature — this is the path that works.

### Seeding a first article (website / apps / title servers)

```sh
newsctl ... article create -slug hello -platforms all \
  -locale "en-US=Hello from OpenPak" -body-text "News is live."
newsctl ... article publish -id <id>
curl 'localhost:20040/v1/articles?platform=switch'
```

Datasets are immutable once sealed; a new dataset with a later
`valid_from` supersedes the old one. Only sealed datasets with a validity
window covering now are served.

## Adapter relationship

The Switch adapter hosts the emulator delivery routes on ports.md 20070 by
mounting the same package (`render/switchbcat`):

```go
import "github.com/openpak/news/render/switchbcat"

svc := switchbcat.NewService(reader, publicBaseURL, logger)
mux.Handle("/api/emulator/v1/bcat/", http.StripPrefix("/api/emulator/v1/bcat", svc.Mux()))
// Renderer (NB-08): svc.Renderer() renders the delivery manifest per title.
```

No console-facing hostnames are mounted; NB-1 (bcat hostnames + SANs) is
withdrawn per the NB-00 finding.

## Operations

- Health: `GET /healthz` (renderer mux), gRPC health service.
- Metrics: `GET :20042/metrics`; pprof under `/debug/pprof/`.
- Logs: slog text on stderr; the service holds no player data.
- Database: Postgres; connection `NEWS_DATABASE_URL`, pool max 8.
- Tests: unit tests always run; store SQL tests run against
  `NEWS_TEST_DATABASE` (ports.md 23001, wired in CI as a service).
- CI: `go vet ./...` and `go test -race ./...` green (`.github/workflows/ci.yml`).

## Status (PRD §10, as corrected)

- N0 spike finding written and **corrected** — console surfaces NO-GO
  (owner review accepted as the record).
- N1' emulator delivery end to end on Citron (article + dataset) —
  tooling ready; hardware gate owner-run.
- N2' LayeredFS placement contract for the setup homebrew — contract
  defined above; implementation lands with the homebrew.
- N3 authoring pages (website agent), read API + emulator delivery — in.
- N4 SpotPass renderer — unchanged (Wii U/3DS, when the first title lands).
