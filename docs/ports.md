# Ports — news (trimmed)

> Trimmed copy for this repository; the canonical document lives at
> `Openpak/ports.md` and governs. Last synchronised 2026-09-15.

One block per concern, nothing below 20000, nothing at or above 27000 (Photon-Reference and
Steam live there). Every service reads its listeners from `<SVC>_HTTP_ADDR`, `<SVC>_GRPC_ADDR`,
`<SVC>_METRICS_ADDR`; the values below are the defaults and the local-run convention. Each
service owns a block of ten: +0 HTTP, +1 gRPC, +2 metrics/pprof, +3..+9 spare.

## 20040 block

| Block | Service | HTTP | gRPC | metrics |
| --- | --- | --- | --- | --- |
| 20040 | `news` | 20040 | 20041 | 20042 |

The native News CDN (`newscdn`, container `openpak-news-experiment`) uses
+0 for its TLS console listener and **+3 (20043) for its plain-HTTP admin
API** (internal network only, `docs/admin-api.md`).

Other services' rows live in the canonical ports.md.
