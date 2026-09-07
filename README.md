# news

One platform-neutral news service for the whole OpenPak network, plus
Switch BCAT delivery (PRD: `prds/news-bcat-prd.md`). An article is written
once — targeted by **platforms, titles, regions, languages** (empty = all)
— and delivered through every channel a player has: the Switch News
channel (BCAT topics), the website, companion apps and title servers via
the read API. Per-title BCAT datasets (schedules, seasonal content) are
Switch-only by nature and live with the Switch renderer.

License: AGPL-3.0-only (`LICENSE`, verbatim from gnu.org). Go 1.26,
module `github.com/openpak/news`.

## Layout (PRD §8)

```
cmd/news               the binary: authoring gRPC, read API, renderer, metrics
cmd/newsctl            operator CLI (stopgap authoring + acceptance journeys)
internal/content       model, audience targeting, read API (NB-03, NB-07)
internal/store         PostgreSQL implementation (pgx), embedded migrations
internal/objects       content-addressed object store (images, dataset files)
internal/grpcapi       authoring gRPC (NB-2, NB-4), operator-key gated
internal/metrics       metrics/pprof listener (ports.md +2)
render/                renderer contract (NB-08): audience in, console-format out
render/switchbcat      Switch channel: topics/list/data handlers + BCAT datasets,
                       emulator delivery (NB-01/02/05/06). The adapter mounts this.
proto/, gen/           news.v1 authoring gRPC
docs/spike-nb00.md     NB-00 spike finding: what the console verifies (go/no-go)
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

Switch renderer (mounted by the Switch adapter on the bcat hosts,
contract request NB-1; standalone HTTP for dev/emulator):

- `GET /api/nx/v1/titles/<title>/topics?c[]=FR&c[]=GB` — News topics (NB-01)
- `GET /v1/list` — titles with served datasets (NB-02)
- `GET /api/nx/v1/titles/<title>/manifest` and `GET /<title>/<path>` — dataset
- `GET /images/<key>` — article images
- Emulator delivery (NB-05): `GET /api/emulator/v1/bcat/titles`,
  `/titles/<title>` (manifest with absolute file URLs),
  `/titles/<title>/files/<path>` — Citron maps `path` into its LayeredFS
  BCAT folder.

All responses carry `ETag`, `Last-Modified`, `Cache-Control` and answer
`If-None-Match` / `If-Modified-Since` (NB-06).

Ports follow `../ports.md` block 20040: HTTP `:20040`, gRPC `:20041`,
metrics/pprof `:20042` (`NEWS_HTTP_ADDR`, `NEWS_GRPC_ADDR`,
`NEWS_METRICS_ADDR`).

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

### Seeding a first article (N1 journey)

An "all platforms" article appears on every title's topics request:

```sh
newsctl ... article create -slug hello -platforms all \
  -locale "en-US=Hello from OpenPak" -body-text "News is live."
newsctl ... article publish -id <id>
curl 'localhost:20040/api/nx/v1/titles/0100ed9024eb8000/topics?c[]=FR&c[]=GB'
```

### Seeding a dataset (N2 journey)

```sh
newsctl ... dataset create -title 0100ed9024eb8000 -until 2027-01-01T00:00:00Z
newsctl ... dataset put -dataset <id> -path event/season.bin -file season.bin
newsctl ... dataset seal -dataset <id>
curl localhost:20040/v1/list && curl localhost:20040/0100ed9024eb8000/event/season.bin
```

Datasets are immutable once sealed; a new dataset with a later
`valid_from` supersedes the old one. Only sealed datasets with a validity
window covering now are served.

## Adapter mounting (contract request NB-1)

The Switch adapter (21000) mounts the renderer; `news` never opens
console-facing ports itself (ports.md rule 2):

```go
import "github.com/openpak/news/render/switchbcat"

svc := switchbcat.NewService(reader, publicBaseURL, logger)
mux.Handle("/", svc.Mux()) // on the three bcat hostnames
// add bcat-*-lp1.cdn.nintendo.net to the certificate SAN list
```

`svc.Renderer()` implements `render.Renderer` (NB-08) if the adapter wants
the channel payload directly.

## Wire shapes and the spike

The topics/list/manifest JSON shapes are the console's format; the exact
field names are being confirmed on hardware (NB-00). The shapes live in
one file each — `render/switchbcat/topics.go`, `data.go` — so the N1/N2
observation protocol in `docs/spike-nb00.md` can adjust them fast. Every
topics request is logged with its raw query for that comparison.

## Operations

- Health: `GET /healthz` (renderer mux), gRPC health service.
- Metrics: `GET :20042/metrics`; pprof under `/debug/pprof/`.
- Logs: slog text on stderr; the store never logs secrets (there are none:
  the service holds no player data).
- Database: Postgres; connection `NEWS_DATABASE_URL`, pool max 8.
- CI: `go vet ./...` and `go test -race ./...` green (`.github/workflows/ci.yml`).
- Integration (Postgres) checks run as unit-level fakes today; the store
  SQL is exercised by the owner journey in this README — run it against a
  scratch database before each deploy.

## Status (PRD §10)

- N0 spike finding written (`docs/spike-nb00.md`) — owner gate pending.
- N1/N2/N3 tooling ready (seed journeys above); hardware gates owner-run.
