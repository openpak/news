# Emulator BCAT delivery

> **Current Switch client (2026-10-13): Harbor**, at
> `~/REPOS/Openpak/emulators/harbor`. Harbor is an independent
> OpenPak project based on Eden. Citron, Ryujinx (desktop and Android), and
> the former OpenPak Eden checkouts have been removed. References to those
> forks below record historical evidence only: do not run their commands,
> reinstall them, or select them for current work. Use Harbor's README and
> current handoff for builds and testing; old results do not establish Harbor parity.

Harbor reads the existing OpenPak emulator BCAT contract at the website
origin. `cmd/bcatserve` hosts operator-supplied dataset snapshots using
`render/switchbcat`; the website relays its three delivery routes when
`BCAT_URL` is configured. The native News CDN is a separate service.

This JSON path supplies emulator HLE caches. The same service also has a
[native BCAT renderer](native-game-bcat.md) for Switch, enabled with its
OpenPak signing key and console verification public key. The native daemon
downloads signed containers and creates its own cache metadata; it does not
consume the JSON path. Physical Switch acceptance remains a separate test.

## Dataset input

Supply each title under a 16-character lowercase title ID:

```
datasets/0100f8f0000a2000/manifest.json
datasets/0100f8f0000a2000/files/vsdata/VSSetting_0.byaml
datasets/0100f8f0000a2000/files/coopdata/CoopSetting.byaml
```

Use the existing manifest fields: `format: 1`, `title_id`, RFC3339
`valid_from` and `valid_until`, and `files` entries with `path`, byte
`size` and lowercase `sha256`. The input's `url` fields are ignored;
the renderer constructs same-origin URLs. Each path is exactly
`directory/filename`, with ASCII letters, digits, underscores, hyphens
or dots and at most 31 characters per component. Dot segments are
rejected. Include every file needed by the game.

This loader requires a finite validity window and verifies all paths,
sizes and hashes before starting. It bounds files to 64 MiB and the
snapshot to 128 MiB. Loaded bytes are immutable for the service's
lifetime, so changing the mounted files cannot cause a manifest/hash
mismatch. Restart to activate a replacement snapshot. Expired and
future datasets are absent from the public index and return 404 for
their manifest and files. The index remains available as `{"titles":[]}`
when no dataset is current.

These checks verify delivery integrity, not game semantics. Validate
BYAML contents, schedule coverage and in-game acceptance independently.
The binary contains no bundled game assets or third-party dataset.
Keep the source URL, retrieval date, archive and per-file hashes, and
authorization/provenance with operator-supplied data outside the code
repository.

## Build and deployment

From `news/`:

```sh
go test -race ./internal/datasetfiles ./render/switchbcat
go vet ./internal/datasetfiles ./cmd/bcatserve
go build -o bcatserve ./cmd/bcatserve
./bcatserve -root /path/to/datasets -check
./bcatserve -root /path/to/datasets -listen 127.0.0.1:20044
```

For the ARM64 host, build with `CGO_ENABLED=0 GOOS=linux GOARCH=arm64`
and `go build -trimpath -o bcatserve ./cmd/bcatserve`. Place the binary
beside `tools/bcatserve/Containerfile`, build the local image
`localhost/openpak-bcat:validated`, and use
`tools/bcatserve/openpak-bcat.container` as the quadlet template.
Its data mount is read-only; root and files must be readable by UID
65532. Port 20044 is internal, in the news block; the template publishes
it only on loopback for host verification. Copy the public key from
`tools/news-patch/openpak-bcat-public.pem` beside the image binary. Mount the
existing OpenPak private signing key as the template's secret; never bake
it into the image. Its UID must match the container's UID 65532.

Deploy the updated website with:

```
BCAT_URL=http://openpak-bcat:20044
```

The website must join the same Podman network. This keeps Harbor's
existing website origin and needs no emulator configuration change.
Unconfigured delivery answers 503; a failed upstream answers 502;
a reachable service without a current dataset answers 404. The proxy
preserves file bytes, hashes, validators and Range requests, and strips
player cookies and authorization credentials from public downloads.

For Switch, configure the updated nx-baas with
`BCAT_INTERNAL_URL=http://openpak-bcat:20044`. It relays native per-title
lists and signed data URLs to this same service, independently of the
existing `NEWS_INTERNAL_URL` configuration.

Check the internal and public index, manifest and every file. Compare
file sizes and SHA-256 to the input manifest; verify 304 responses with
`If-None-Match`. For a clean emulator cache, launch the title and verify
that files are fetched, installed and read through BCAT before checking
account login and matchmaking. A prefilled cache alone does not verify
automatic delivery.

For rollback, restore the website's previous image and configuration,
then stop the new BCAT service. Retain previous dataset snapshots and
images. No native News route or console-facing hostname needs changing.

## Splatoon 2 investigation, 2026-10-12

The ROG3 trace with an empty BCAT cache showed internet acceptance and
account availability, but missing `vsdata`, `coopdata` and `fesdata`
prevented the game from reaching socket/SSL activity. The website
returned generic 404 for both the index and title manifest. Production
was running the native News CDN, not the per-title dataset renderer.

With the owner's explicit authorization, a public prior third-party reference BCAT
package was downloaded as a local test fixture. Seven unmodified game
data files were installed in the previously empty HLE cache; no prior reference
implementation code was copied. All four BYAML files parsed,
and both VS and co-op schedules covered the test date. The game then
read the files, authenticated with OpenPak, entered the lobby and
started a Turf War player search. A completed multiplayer battle and
long-term Vulkan stability remain unverified.

Detailed archive provenance, hashes, captured screenshots and phone
logs are owner-local under
`scratch/harbor-eden/tmp/splatoon2-rog3-20261012/lobby-network/bcat-package/`.
The package contains prior-reference-stack-authored festival strings as
well as binary game assets; it is not labeled OpenPak-authored data or
bundled with this service. Hosting a dataset is a separate operator
action from deploying this code.
