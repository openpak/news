# News Server

OpenPak's News Server.

| | |
| --- | --- |
| Network | All OpenPak networks (platform-neutral); Switch emulator BCAT delivery |
| Role | News service: articles targeted by platform, title, region and language, plus per-title BCAT datasets for emulators |
| Depends on | PostgreSQL; [`nx-baas`](https://github.com/openpak/nx-baas) (optional push target) |
| Used by | OpenPak website, apps and title servers (read API) |
| Image | `ghcr.io/openpak/news` |
| Licence | AGPL-3.0-only |

## What it serves

| Endpoint | What |
| --- | --- |
| `news.v1.News` gRPC (operators only) | Authoring: articles, images, datasets |
| `GET /v1/articles` | Read API for the website, apps and title servers |
| `GET /api/emulator/v1/bcat/...` | Per-title Switch BCAT delivery manifests and dataset files for emulators |
| `GET /images/<key>` | Article images referenced by the read API |
| Internal admin API (`newscdn -data`) | Website dashboard publishing, internal network only |

## Run

```sh
cp example.env .env  # fill in; never commit real values
go run ./cmd/news
```

## Docs

- [Protocol](docs/protocol.md)
- [Evidence](docs/evidence.md)
- [Deploy](docs/deploy.md)
- [Notes](docs/notes.md)
- [Admin API](docs/admin-api.md)
- [Emulator BCAT delivery](docs/emulator-bcat-delivery.md)
- [Native game BCAT](docs/native-game-bcat.md)
- [Native News agent handoff](docs/native-news-agent-handoff.md)
- [Native News article delivery](docs/native-news-article-delivery.md)
- [Native News authoring guide](docs/native-news-authoring-guide.md)
- [Native News catch-all](docs/native-news-catchall.md)
- [Native News container](docs/native-news-container.md)
- [Native News local format](docs/native-news-local-format.md)
- [Native News probe 01](docs/native-news-probe-01.md)
- [Native News reopen](docs/native-news-reopen.md)
- [NX BCAT split](docs/nx-bcat-split.md)
- [Provenance](docs/provenance.md)
- [NB-00 spike](docs/spike-nb00.md)
- [NB-00 spike review](docs/spike-nb00-review.md)

## Licence

AGPL-3.0-only. See [LICENSE](LICENSE).
