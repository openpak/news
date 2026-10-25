# Protocol: news

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

`cmd/bcatserve` can also serve validated operator-supplied filesystem
snapshots without the content database. The website's `BCAT_URL` relay
keeps these routes at Harbor's configured origin. See the
[delivery runbook](emulator-bcat-delivery.md), including the separate
real-console requirements.

- `GET /api/emulator/v1/bcat/titles` — titles with served datasets
- `GET /api/emulator/v1/bcat/titles/<title>` — delivery manifest
  (paths, sizes, sha256, validity window, absolute file URLs)
- `GET /api/emulator/v1/bcat/titles/<title>/files/<path>` — dataset file
- `GET /images/<key>` — article images referenced by the read API

All responses carry `ETag`, `Last-Modified`, `Cache-Control` and answer
`If-None-Match` / `If-Modified-Since` (NB-06).

Datasets are immutable once sealed; a new dataset with a later
`valid_from` supersedes the old one. Only sealed datasets with a validity
window covering now are served.

## Native News admin API (`newscdn -data`)

`cmd/newscdn` serves the signed console tree on its TLS listener (`:20040`).
With `-data DIR` it also runs the admin API from
[`admin-api.md`](admin-api.md) on a separate plain-HTTP listener
(`-admin-listen`, default `:20043`, internal network only):

- `DIR/channels/` is the source of truth, in the `channels/` file format
  (`lineup.json`, `retired.json`, `<topic>/<id>.json`, plus uploaded
  `screenshots/<id>.png|jpg` and `icons/<topic>.png|jpg`). On first start
  with an empty `DIR`, `-import /channels` copies a `channels/` tree in
  (`-import-screenshots` also copies the third-party captures, off by
  default); `-import-manifest` takes the last file-based build's
  `manifest.json` for overwrite continuity.
- `POST /admin/v1/publish` runs `internal/newsseed` (the code behind
  `cmd/newsseed`) into `DIR/builds/<n>/seed`, starting from the newest
  build's `manifest.json`. It then signs every payload in-process (the
  `sign-seed.sh` logic in Go) and verifies each container against the
  committed console key. The result goes to `DIR/builds/<n>/signed`,
  which it loads and swaps in atomically. Then it POSTs `{topic_id,
  revision}` to nx-baas for every topic whose list changed. The last
  `-keep-builds` (10) builds are kept for `POST /admin/v1/rollback`.
- Until the first publish, the static `-dir` tree is served.

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

## Adapter relationship

An adapter can host the emulator delivery routes on ports.md 20070 by
mounting the same package (`render/switchbcat`) with the full route path:

```go
import "github.com/openpak/news/render/switchbcat"

svc := switchbcat.NewService(reader, publicBaseURL, logger)
mux.Handle("/api/emulator/v1/bcat/", svc.Mux())
// Renderer (NB-08): svc.Renderer() renders the delivery manifest per title.
```

No console-facing hostnames are mounted; NB-1 (bcat hostnames + SANs) is
withdrawn per the NB-00 finding.
