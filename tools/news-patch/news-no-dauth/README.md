# News memory-download edge-token omission experiment

This separate 21-byte IPS is for BCAT build
`6D9772A733E2370B3F401EFEB7CA8F664309BD66000000000000000000000000`
from the owner's HOS 22.5.0 firmware. It replaces eight instruction bytes at
mapped `0xfc7a4` (uncompressed NSO offset `0xfc8a4`). It leaves an empty token
and skips the call to the token helper before starting the News memory download.

This is an OpenPak transport experiment, not proof of a working catalog. It
affects this News downloader for every URL, not just the OpenPak hostname.
Nintendo endpoints that require the token will not authenticate these requests.
Use the OpenPak DNS configuration. The global dauth service and shared token
helper used by other BCAT paths are not modified. The separate retail BCAT
verification-key patch still has its previously documented game BCAT effects.

## Install and remove

Use [probe v0.19](../../news-probe/README.md): **D-pad Right** installs this
patch after checking the running BCAT build. Reboot Atmosphere, then **ZL**
tests native catalog retrieval. Keep the existing verification-key patch.
**D-pad Left** removes this token patch; reboot to restore token acquisition.
R/L continue to manage only the verification-key patch.

Manual path:
`sd:/atmosphere/exefs_patches/openpak_news_no_dauth/6D9772A733E2370B3F401EFEB7CA8F664309BD66000000000000000000000000000000.ips`.
Deleting that file and rebooting also rolls back, including via an SD reader
if the console cannot boot. Applies to matching Atmosphere boots using the SD.
Other patch directories are not audited for conflicts by the installer.

IPS SHA-256: `4657c3d4fbc3442fc1c38a29e2db6548c2f71d4688d0e804acce4885083a3561`.

## Evidence

The memory-download worker at `0xfc5d0` calls `0xfc760` before it calls the
curl setup path. `0xfc760` calls `0xfecc0` at `0xfc7a8`. That helper calls
`0x90f70`, then `0xb59f0` / `0xb5af0`, which obtains a `dauth:0` session and
dispatches command 10 via `0xb60a0` (command immediate at `0xb60fc`).
Command 11 loads the resulting token via `0xb6150`. These match the documented
[EnsureEdgeTokenCacheAsync and LoadEdgeTokenCache APIs](https://switchbrew.org/wiki/Account_services#dauth:0).
The later HTTP setup formats `X-Nintendo-DenebEdgeToken: %s` at `0xfd7e0`.

The token error propagates unchanged through this path, apart from specific
cancellation translations. The subsequent curl error conversion table does
not contain 0x167B. This establishes a pre-download authentication dependency
consistent with the observed failure, not the precise failing instruction in
the external authentication service. The earlier local device-PKI failure
remains a related lead, not a proven internal dauth failure site.

The replacement is `strb wzr,[x21]; mov w0,wzr`, instead of argument setup
and `bl 0xfecc0`. The output token is explicitly NUL-terminated; state cleanup,
locks, cancellation branch and following HTTP/container processing remain.
Only one direct caller of `0xfc760` was found, at `0xfc610`. Other direct
call sites of the shared helper `0xfecc0` remain unchanged.

## Validation and reproduction

From the news module:

```sh
go run ./cmd/newspatch -nso /private/bcat-uncompressed.nso \
  -news-no-dauth -out /tmp/openpak-news-no-dauth.ips
go test -race ./cmd/newspatch
go vet ./cmd/newspatch
```

The generator verifies the full original NSO hash, build ID and both original
instructions; it refuses modified inputs and existing output files. Tests
check exact patch scope and rejection cases. Applying the IPS to the actual
owner-local NSO confirmed only the eight-byte call site changes; the generated
IPS matches the NRO's embedded bytes. ARM64 Unicorn execution of the original
worker with a simulated 0x167B token failure returns failure, while the patched
worker returns success with an empty token and no token-helper call. The
already-cancelled branch still returns failure. Mutex calls remain balanced
in all three executions. This emulation does not model dauth, networking or
the complete system. The v0.19 NRO builds with -Wall -Wextra -Werror.

Hardware result: installation succeeded, and after reboot v0.19 ZL returned
success with 155 decoded bytes. The server logged the native catalog response
at 18:49:45 UTC. Exported plaintext comparison and Find Channels UI validation
remain pending; this does not establish subscription delivery.
