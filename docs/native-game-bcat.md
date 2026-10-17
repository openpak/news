# Shared game BCAT service

> **Current Switch client (2026-10-13): Harbor**, at
> `~/REPOS/Openpak/emulators/harbor`. Harbor is an independent
> OpenPak project based on Eden. Citron, Ryujinx (desktop and Android), and
> the former OpenPak Eden checkouts have been removed. References to those
> forks below record historical evidence only: do not run their commands,
> reinstall them, or select them for current work. Use Harbor's README and
> current handoff for builds and testing; old results do not establish Harbor parity.

`bcatserve` owns one validated snapshot per title. The existing
`render/switchbcat` produces Harbor's JSON manifests and raw file downloads;
`render/nativebcat` produces native game BCAT lists and signed file containers
from the same `content.Dataset` and object bytes. No Splatoon-specific branch
or game-data generator is part of either renderer. Adding another game's
dataset needs a title manifest, its actual files, and game acceptance testing.

The website relays Harbor requests through `BCAT_URL`; nx-baas relays native
`nx_data_<title>` lists and `/bcat/` data requests through `BCAT_INTERNAL_URL`.
Both point to `http://openpak-bcat:20044`. Native News stays with its existing
News renderer and authoring service; game BCAT does not take over topic
subscriptions or OpenPak News lists.

## Native wire facts and provenance

Primary public sources:

- [BCAT container format](https://switchbrew.org/wiki/BCAT_Content_Container):
  0x120-byte header; magic `bcat`; version 1; RSA-2048; signed plaintext is
  permitted when crypto type is outside 1..3; AES-CTR types are 1..3.
- [BCAT hosts and list route](https://github.com/kinnay/NintendoClients/wiki/BCAT-Servers).

Owner-local 23.0.1 BCAT analysis, project
`scratch/harbor-eden/research/g3pctl-news/ghidra/bcat23`:

- Native list parser's string callback (function containing 0x859c8b8,
  0x859cc34 and 0x859c910) accepts directory `name`, `mode`, `digest` and
  per-file `filename`, `url`, `digest`. The supported modes are `sync` and
  `copy`; this renderer uses `sync` for a complete directory snapshot.
- Integer callback sites 0x859c434, 0x859c484 and 0x859c4c4 identify
  `required_application_version`, `data_id` and `size`.
- Digest parser `FUN_085a18f0` parses two `%016llx` words into a 16-byte
  native structure. The renderer writes each little-endian u64 as hex,
  so the parsed structure contains the intended raw digest bytes.
- URL copy loop caps its stored string at 159 characters; startup validation
  rejects longer generated file URLs.
- Earlier owner-local 22.5.0 field/route observations are retained in
  `nx-baas/docs/firmware-22.5.0-followup-2026-09-12.md`, section 21.

No Nintendo decompiled source is placed in the repositories. The analysis
records observable format facts; the Go renderer is independently written
against OpenPak's existing content and MessagePack/container packages.

The native file digest uses MD5 of decoded file bytes, matching the 16-byte
BCAT delivery-cache digest used by Harbor. **Inference needing hardware
confirmation:** the native downloader compares this decoded digest rather
than a digest of the outer container. The directory digest is a revision
marker derived from the ordered file descriptors. The native parser accepts
the 16-byte value; whether firmware independently recomputes a directory
digest remains unconfirmed. File `size` includes the outer header, following
the download-size interpretation used by the native News renderer.
These details must be checked on 23.0.1 before claiming console acceptance.

`data_id` is an opaque u64 revision derived from the file SHA-256; it changes
when the payload changes. `required_application_version` is zero: the BCAT
service introduces no game-update requirement. The snapshots themselves
must still be compatible with the game's installed version.

## Container signing

Native game delivery uses crypto type **0**, RSA PKCS#1 v1.5/SHA-256 and
secret index 0: signed plaintext containers. These public files do not need
confidentiality, and the documented plaintext mode avoids holding each
title's NACP passphrase and firmware salt in the service. The signature is
fully verified; no signature check is bypassed. Existing native News
encrypted containers continue to use their existing AES configuration.

Native startup requires both `-signing-key` and `-verify-key`, checks they
match, then prebuilds every current native dataset. The supplied public key
must be the one installed by OpenPak's BCAT verification-key patch. That
patch is already bundled by the setup NRO for the 23.0.x BCAT build
`84EF6747BB15A01707B883B29A2A1CE5FC1734C2000000000000000000000000`;
an unpatched retail verifier will reject OpenPak-signed containers.
No key material is bundled in the service image: only the public key is
copied alongside the binary, and the private key is mounted as a secret.

Native files currently inherit the container encoder's 4 MiB payload limit.
Harbor's input limit remains 64 MiB per file. A currently published dataset
that exceeds native limits rejects native startup rather than publishing
an incomplete dataset. Native data URLs name the title and a content
revision. Lists and files carry ETag and support conditional and Range
requests; expired datasets stop serving native files and yield a signed
empty list. Unknown games likewise get a signed empty list.

## Validation status

Host tests verify plaintext-container signatures and corruption rejection,
shared raw/signed payload identity, native metadata fields, digest word
encoding, conditional and Range responses, and expiry. nx-baas tests
verify that both native list/file paths preserve container bytes and cache
headers while News paths keep their existing routing.

Splatoon 2 was verified on Harbor through lobby entry and Regular Battle
search using the dataset copied at the owner's request. A completed battle,
physical Switch native download/cache installation, and native game
acceptance are separate outstanding checks. A server-side format test is
not a substitute for them.

Deployed 2026-10-12 with the owner's approval. The website and native TLS
adapter were checked from outside the host: all seven raw/signed payload
pairs match, RSA signatures verify against the committed public key,
native Range responses work, raw conditional requests return 304, and an
unknown title receives a signed empty native list. The native adapter's
new data-host certificate verifies against the OpenPak CA.

The deployed local image tags are `localhost/openpak-bcat:20261012`,
`localhost/openpak-website:bcat-20261012` and
`localhost/openpak-nx-baas:bcat-20261012`. Website and adapter registry
auto-update entries are removed from these pinned quadlets so a release
without the relay cannot overwrite the deployment. Restore registry
auto-update when the changes have been included in the released images.
The previous active images are tagged `before-bcat-20261012`, and their
exact quadlets and image IDs are saved under `~/openpak/deploy/bcat/`.
No data volume or native News configuration was replaced.
