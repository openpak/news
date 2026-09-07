# NB-00 review (owner-side, 2026-09-07) — the go/no-go must be reversed for the console

`spike-nb00.md` concludes "TLS is the verification boundary" and marks console News and
per-title BCAT as GO. Two public sources contradict that, and the second is decisive.

## Evidence

1. Kinnay wiki, BCAT-Servers, `GET /api/nx/v1/titles/<id>/topics`: "On success, a **bcat
   content container** containing the topics for the given title id is returned." The
   response is not JSON; it is a container file.
2. switchbrew, "BCAT Content Container" (https://switchbrew.org/wiki/BCAT_Content_Container),
   header 0x120 bytes: magic `bcat`, crypto type (1–3 = AES-128/192/256-CTR, anything else =
   plaintext), RSA/hash type (PKCS1 or PSS over SHA-1 or SHA-256), secret-data index, base
   IV, and an **RSA-2048 signature at 0x20**. The AES key is PBKDF2-HMAC-SHA256 over the
   title's passphrase (control.nacp @ 0x3100) with a salt built from the title id plus a
   secret string held by the sysmodule; the RSA modulus is selected by the `bcat`
   `production_mode` setting, i.e. a Nintendo public key embedded in the sysmodule.
3. switchbrew, BCAT services, News downloader (cmd 30500): "Downloaded files need to be
   encrypted BCAT Content Container (Crypto type 1-3)." News payloads go through the same
   container and the same signature check.
4. Emulator side (Citron, GPL): the bcat service takes the passphrase from the game
   (`SetPassphrase`) and its backend is a null backend; emulators do not verify container
   signatures. Evidence only: `src/core/hle/service/bcat/`.

## Corrected go/no-go

| Surface | Decision | Basis |
| --- | --- | --- |
| Console News (topics) | **NO-GO** without Nintendo's private key or a console-side patch; the project refuses patches | 1, 2, 3 |
| Console per-title BCAT via the bcat hosts | **NO-GO**, same reason | 2 |
| Per-title data on console via LayeredFS placement (setup homebrew writes files to the SD; bcat delivery cache not involved) | **GO**, outside this repo's console handlers | the third-party setup precedent (hosts/LayeredFS evidence) |
| Emulator delivery (NB-05) | **GO** | 4 |
| Website / apps / title-server read API (NB-07) | **GO** | independent of consoles |

## What this means for the code

- `render/switchbcat/topics.go` and `data.go` emit invented JSON. A stock CFW console will
  not parse them. They are not "shapes to adjust on hardware"; the wire format is a signed
  container we cannot sign. Keep the renderer interface, drop the claim that the console
  surface is deliverable, and mark those handlers as emulator/dev-only.
- `bcat-list` route is `/api/nx/v1/list/<name>` per Kinnay, not `/v1/list` (moot for the
  console, but the emulator delivery should not invent a route either).
- N1 and N2 as written cannot pass. Replace them with: N1' emulator delivery end to end on
  Citron (article + dataset), N2' LayeredFS placement contract for the setup homebrew.
- `internal/store` has no tests; add a Postgres-backed test (port 23001) before the
  migration is relied on.

Nothing else in the repo is affected: content model, targeting, read API, authoring gRPC,
CLI, ports and licence are sound and the 15 tests are race-clean.
