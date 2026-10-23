# Native News agent handoff

**Current deployment (2026-09-10):** owner-authorized v006/cdn-v003 now serves four channels, populated archives, twelve summaries and linked full articles. Host and public HTTPS verification pass; native rendering and automatic subscription delivery remain pending. See [current delivery, evidence and rollback](native-news-article-delivery.md). Earlier empty-archive/preparation statements below are historical.

Updated 2026-09-10; hardware evidence through 2026-09-09. Work from `~/REPOS/Openpak/news`, its own Git
repository. The parent directory is not a Git repository. Inspect existing
uncommitted work before editing; several tasks have changes here.

## Ready-to-use agent task

> Read `docs/native-news-authoring-guide.md`, `docs/native-news-container.md`,
> `../prds/news-bcat-prd.md` and `examples/native-news/README.md`. Continue native
> Switch News channel and article delivery. Patched HOS 22.5.0 hardware now
> downloads and decodes the catalog through the native service. Server v004's
> empty online_archives response is deployed; channel-page acceptance,
> subscriptions and server article delivery remain unverified. Check the latest
> owner report and isolated News logs first. Validate the channel page, implement
> the verified archive/article response schema and one original article, then
> connect maintainable channel authoring and a Switch renderer to the existing
> platform-neutral content service. Include update/unpublish/retirement behavior
> so tests do not accumulate. Do not restart solved TLS work, regenerate signing
> keys, modify nx-baas or copy Nintendo content into the repository. Run the
> required checks, update the guide and give the owner concrete hardware tests.
> Do not mark delivery complete based only on a successful catalog download.

## Implementation sequence

1. Obtain the v004 channel-page result. The earlier error coincided with repeated
   unhandled online_archives requests. The new signed response contains
   `{na_required: false, data_list: []}`. It has not yet passed a reported UI test.
2. Validate populated `data_list` entries and their download URL handling.
   Owner-local qlaunch formats the archive URL at `0x653ea8`, parses its map at
   `0x6540a8` and starts array entries around `0x654178`. Investigate required
   fields and `/api/nx/v2/latest_news/<name>` as code/logs warrant. Do not assume
   archive entries contain full articles or invent URL/pagination fields.
3. Link one fresh original article to `openpak_news` and serve its container.
   Verify the stored identity and native title/body/images. The sample follows
   the proven local layout; remote article requirements may differ.
   Progress: the four-channel lineup and twelve opening articles are authored
   in `channels/` and their containers signed and host-verified (2026-09-10);
   deployment and hardware validation await owner review. No article is
   served anywhere until the archive entry schema is traced.
4. Add stable channel IDs, localized metadata, icons, publication state,
   membership and targeting to the existing service/store patterns. Define
   versioned channel contracts and migrations; no native channel CRUD exists yet.
5. Render published platform-neutral content into signed News objects. Replace
   hardcoded fixture routing with validated channel/article mappings. Keep keys
   out of the serving image. Coordinate additional adapter hosts/routes separately.
   Progress: fixture routing is no longer hardcoded — the CDN derives per-channel
   routes from the fixture tree, and the payload encoders live in the shared
   `internal/newsformat` package, ready for a store-driven renderer.
6. Verify subscribe/unsubscribe, subscription persistence across reboot, refresh
   delivery, updates and retirement without affecting unrelated articles.

## Code map

| Location | Responsibility |
| --- | --- |
| `examples/native-news/` | Editable JSON and offline MessagePack/JPEG builder |
| `channels/` | The OpenPak channel lineup seed: four channels, opening articles, inventory and seed commands |
| `cmd/newsseed/` | Deterministic seed builder for `channels/` (payloads, artwork, manifest, retire/unretire) |
| `internal/newsformat/` | MessagePack payload shapes hardware has accepted (catalog/detail/archives, NORMAL article) |
| `cmd/newsfixture/main.go` | Original fixed fixtures and proven v0.12 local article layout |
| `cmd/newscontainer/`, `internal/bcatcontainer/` | Container encoding with supplied keys |
| `cmd/newscdn/`, `internal/newsexperiment/` | Isolated HTTPS fixture server; routes derived per channel from the signed fixture tree |
| `cmd/newspatch/`, `tools/news-patch/` | Pinned patches, guards, rollback |
| `tools/news-probe/` | v0.19 hardware probe and patch installer |
| `internal/content/`, `internal/store/`, `internal/grpcapi/` | Platform-neutral service |
| `proto/news/v1/news.proto`, `cmd/newsctl/` | Existing operator contract and CLI |
| `tools/news-cdn/sign-seed.sh` | Signs a seed build owner-privately; verifies every container against the public key |
| `render/switchbcat/` | Per-title BCAT renderer, not a complete native News renderer |
| `../console/openpak-nro/source/hosts.c` | Exact News topics DNS redirect |

## Owner environment

Remote: `<prod-host>`, ARM64/rootless Podman. Existing SSH access:

```sh
ssh -o BatchMode=yes \
  -o StrictHostKeyChecking=yes <prod-host> \
  'journalctl --user -u openpak-news-experiment.service --no-pager -o cat -n 40'
```

Active image: `localhost/openpak-news-experiment:v004`; service:
`openpak-news-experiment.service`; container: `openpak-news-experiment`.
Context: `~/openpak/deploy/news-experiment/`; fixtures: `cdn-v001/`; TLS: `tls/`.
The fixture directory retains its v001 name after v004's archive addition.
Quadlet: `~/.config/containers/systemd/openpak-news-experiment.container`.
Traefik: `~/openpak/deploy/traefik/dynamic.yml`, exact topics SNI at priority
1000, TCP passthrough to port 20040. Other routes are owned elsewhere.
Unknown paths proxy to the existing adapter at port 21000 with CA verification.
Never dump raw adapter request headers; they can contain credentials.

Build using `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath` and
`tools/news-cdn/Containerfile`. Prepare fixtures before activating a new image
tag; update only the experimental quadlet and restart that service. Check public
HTTPS bytes using the existing public CA. Preserve the old image for rollback.
For full rollback remove only the experimental router/service entries from the
current Traefik file and stop the experiment; do not restore a whole old config
over changes from other agents. Deployment evidence is in the container runbook.

Owner-private assets: `~/REPOS/Openpak/.secrets/news/` contains the existing
RSA signing key, prederived AES key, firmware and analysis. These are not samples
to distribute. `bcat-memory.bin` and `qlaunch-memory.bin` use mapped addresses;
uncompressed NSOs retain headers. The tested IPS offsets add 0x100 to the mapped
text/modulus locations. Do not print or commit secret values.
The owner copies reports to `~/report.txt`; they append, so separate
runs by version and epoch. MTP reads here have failed; use copied exports.

## Acceptance and limits

Run the offline sample and independently decode its MessagePack/images. For
code milestones run `go test -race ./...` and `go vet ./...`; database integration
tests require `NEWS_TEST_DATABASE`. Record missing integration coverage. NRO
build details are in `tools/news-probe/BUILD.md`.

Acceptance requires finding/opening the channel, subscribing, receiving an
original server article with body/images, retaining the subscription after
reboot, observing an update and retiring the test. Verify unsubscribe behavior.
PostLocalNews success and article counts alone are insufficient evidence.

The owner authorized both patches and this isolated News host experiment.
That does not extend to unrelated services or broader hostname takeover.
No custom sysmodule is needed for the demonstrated path.
