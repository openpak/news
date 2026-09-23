# News list-fetch edge-token omission experiment

This 34-byte IPS is for BCAT build
`6D9772A733E2370B3F401EFEB7CA8F664309BD66000000000000000000000000`
from the owner's HOS 22.5.0 firmware (`010000000000000c`). It skips the edge-token
helper before the News **list** fetch and the **titles/topics** auto-subscribe
fetch, the same way the separate [`news-no-dauth`](../news-no-dauth/README.md)
patch skips it before the News memory download.

The News worker for a subscribed topic requests
`bcat-list-%.cdn.nintendo.net/api/nx/v1/list/<topic>` and, on title launch,
`titles/<tid>/topics`. Each call obtains a Nintendo edge token first and aborts
if it fails. Against the OpenPak server, which issues no such token, both would
abort before the request is sent. This patch leaves an empty token and lets the
request proceed. It is a transport experiment, not proof of a working catalog:
the server still needs to serve a correctly signed and encrypted list container.

This affects only these two News fetch paths on the matching build. The shared
`dauth` service and edge-token helper are not modified; other BCAT paths still
acquire a token. Use the OpenPak DNS configuration. Keep the verification-key
patch and the `news-no-dauth` memory-download patch installed as well.

## Install and remove

Manual path (matching the delivery `patches.c` uses for the other News patches):
`sd:/atmosphere/exefs_patches/openpak_news_list_no_dauth/6D9772A733E2370B3F401EFEB7CA8F664309BD66000000000000000000000000.ips`.
Deleting that file and rebooting rolls back, including via an SD reader if the
console cannot boot. The patch is selected by NSO build id, not a firmware
version string, so a console on another build simply ignores it.

IPS SHA-256: `9524b87208ae8a7627d92738faf0196440e53f3401de00b21eed9317b39026c1`.

## Evidence

Five direct callers of the edge-token helper (`0xfecc0`, Ghidra `0x1fecc0`)
exist in this module: `0xfc7a8` (memory download, already covered by
`news-no-dauth`), `0x113f9c` (titles/topics fetch), `0x116894`, `0x116ef0` and
`0x117284` (the `/v1/list/` fetch, in `FUN_00217210`). Addresses are mapped
addresses; the Ghidra project used base `0x100000`, and IPS offsets are the NSO
offsets (mapped + `0x100` for the NSO header), following
[Atmosphère's loader patch handling](https://github.com/Atmosphere-NX/Atmosphere/blob/master/stratosphere/loader/source/ldr_patcher.cpp).

Both patched sites share the memory-download site's shape: a `0x401`-byte token
buffer is `memset` to zero, the helper is called with the buffer pointer in
`x0`, and the module branches away (`cbnz w0`) to an abort path on a non-zero
result.

List fetch, mapped `0x117280`–`0x117287` (NSO `0x117380`):

```
117278: add  x0, sp, #0x37        ; x0 = token buffer (already memset to 0)
11727c: add  x1, sp, #0x438
117280: add  x2, x20, #0x78       ; -> strb wzr, [x0]   (NUL-terminate token)
117284: bl   0xfecc0              ; -> mov  w0, wzr      (report success)
117288: cbnz w0, 0x117450         ; w0 == 0 -> proceed to the request
```

Titles/topics fetch, mapped `0x113f98`–`0x113f9f` (NSO `0x114098`):

```
113f90: add  x0, sp, #0x167       ; x0 = token buffer (already memset to 0)
113f94: add  x1, sp, #0x568
113f98: mov  x2, x24              ; -> strb wzr, [x0]
113f9c: bl   0xfecc0              ; -> mov  w0, wzr
113fa0: cbnz w0, 0x11403c         ; w0 == 0 -> proceed
```

Original bytes verified in `bcat/main.elf`:
`82 e2 01 91 8f 9e ff 97` (list) and `e2 03 18 aa 49 ab ff 97` (titles/topics).
Replacement at both sites: `1f 00 00 39 e0 03 1f 2a`
= `strb wzr, [x0]` (`0x3900001f`) then `mov w0, wzr` (`0x2a1f03e0`).

This differs from `news-no-dauth` only in the buffer register (`x0` here,
`x21` there, so `strb wzr,[x0]` rather than `strb wzr,[x21]`) and the branch
sense after the call (`cbnz` here, `cbz` there); both proceed when `w0 == 0`.
The `memset` already zeroes the buffer, so the `strb` is a defensive
NUL-terminate. No token helper is called, no other register the caller depends
on afterward is clobbered (`x2` is recomputed on the taken path), and the abort
branch is left in place for real errors on unpatched builds.

The two other helper call sites (`0x116894`, `0x116ef0`) are left unchanged:
they are not on the OpenPak list/subscription path targeted here.

This static reasoning does not, on its own, prove native list acceptance; that
still needs a hardware round-trip after the status-2 subscription fix.
