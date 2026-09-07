# NB-00 Research spike — finding (CORRECTED 2026-09-07)

The original conclusion of this spike ("TLS is the verification boundary",
GO for console News and console BCAT) was **wrong** and is reversed by the
owner-side review with citations: see
[spike-nb00-review.md](spike-nb00-review.md). This file is kept as the
spike's record; the review is authoritative.

## Corrected finding

The topics response is not JSON. Per Kinnay (BCAT-Servers) it is a **bcat
content container**; per switchbrew (BCAT Content Container) that is a
0x120-byte header with an RSA-2048 signature at 0x20 and optional AES-CTR
encryption keyed from the title's passphrase (NACP 0x3100) plus a secret
held by the bcat sysmodule, with the RSA modulus selected by the sysmodule's
`production_mode` setting — a Nintendo key we do not have. The News
downloader (BCAT cmd 30500) requires the same encrypted, signed container.
Therefore no payload we emit can be parsed by a stock CFW console, and the
project's standing rule excludes console-side patches.

The documented list route is `/api/nx/v1/list/<name>` (not `/v1/list`),
recorded here as evidence only.

## Go/no-go (final, per the review)

| Surface | Decision |
| --- | --- |
| Console News (topics) | **NO-GO** (needs Nintendo's private key or a patch) |
| Console per-title BCAT via bcat hosts | **NO-GO** (same container, same key) |
| Per-title data on console via LayeredFS placement (setup homebrew, SD) | **GO** — outside this repo's console handlers (N2' placement contract) |
| Emulator delivery (NB-05) | **GO** — Citron takes the passphrase from the game and verifies nothing |
| Read API for website / apps / title servers (NB-07) | **GO** — console-independent |

## Consequences applied in this repo

- The invented console-format handlers were removed; `render/switchbcat`
  now serves only the emulator delivery endpoints and implements
  `render.Renderer` as the delivery manifest renderer. Contract request
  NB-1 (adapter mounts handlers on the bcat hostnames) is **withdrawn**.
- Milestones N1/N2 are replaced by N1' (emulator delivery end to end on
  Citron) and N2' (LayeredFS placement contract for the setup homebrew);
  see README "Status".
- Store SQL is covered by a Postgres-backed integration test
  (`NEWS_TEST_DATABASE`, port 23001 per ports.md).
