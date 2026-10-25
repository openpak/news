# Deploy: news

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

## Ports

Ports follow the port plan (https://docs.openpak.org/contributors/ports/) block 20040: HTTP `:20040`, gRPC `:20041`,
metrics/pprof `:20042` (`NEWS_HTTP_ADDR`, `NEWS_GRPC_ADDR`,
`NEWS_METRICS_ADDR`).

## Native News admin API configuration

- The image needs `cjpeg` (libjpeg-turbo), so `tools/news-cdn/Containerfile`
  is now Alpine-based.

| Variable | Meaning |
| --- | --- |
| `NEWS_ADMIN_KEY` | Required `X-Internal-Key` for the admin API. Unset = every admin request answers 503 |
| `NEWS_SIGNING_KEY` | Path to the OpenPak BCAT RSA-2048 private key PEM (secret file). Must match `tools/news-patch/openpak-bcat-public.pem` |
| `NEWS_AES_KEY` | Path to the raw News AES key (secret file) |
| `NX_BAAS_PUSH_URL` | nx-baas push route, e.g. `http://openpak-nx-baas:20070/internal/news/push`. Unset = no push |
| `NX_BAAS_INTERNAL_KEY` | nx-baas `X-Internal-Key` |
| `NX_BAAS_PUSH_WAIT_RANGE` | Optional `wait_range` seconds sent with each push |

Keys are read from their files at publish time and never logged or returned.
The website needs `NEWS_ADMIN_URL=http://openpak-news-experiment:20043` and
the same `NEWS_ADMIN_KEY`, and must be on the `openpak` network.

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

### Seeding a first article (website / apps / title servers)

```sh
newsctl ... article create -slug hello -platforms all \
  -locale "en-US=Hello from OpenPak" -body-text "News is live."
newsctl ... article publish -id <id>
curl 'localhost:20040/v1/articles?platform=switch'
```

## Operations

- Health: `GET /healthz` (renderer mux), gRPC health service.
- Metrics: `GET :20042/metrics`; pprof under `/debug/pprof/`.
- Logs: slog text on stderr; the service holds no player data.
- Database: Postgres; connection `NEWS_DATABASE_URL`, pool max 8.
- Tests: unit tests always run; store SQL tests run against
  `NEWS_TEST_DATABASE` (ports.md 23001, wired in CI as a service).
- CI: `go vet ./...` and `go test -race ./...` green (`.github/workflows/ci.yml`).
