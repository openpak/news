# NB-00 Research spike — what the console verifies on News and BCAT payloads

Status: finding written 2026-09-07, hardware observation **pending owner
run** (the N0 gate is the owner accepting the go/no-go below). This spike is
blocking per PRD §7; N1/N2 execute the observation protocol it defines.

## Evidence available today

1. Console log lines (2026-09-07, owner's CFW Switch with the OpenPak CA
   and the third-party setup redirect active):
   `GET bcat-topics-lp1.cdn.nintendo.net/api/nx/v1/titles/<title>/topics?c[]=FR&c[]=GB`
   fires on title launch for Outbound (`0100ED9024EB8000`) and a system
   title. Our side returned 404; the console logged it and moved on. Two
   facts follow:
   - the console **reaches our TLS endpoint** through the adapter and
     accepts the OpenPak certificate for these hosts;
   - the request is **unauthenticated** at the HTTP layer (no token
     headers reported), filtered by country only.
2. Public documentation (Kinnay wiki, "BCAT-Servers"): the three bcat hosts
   (`bcat-list`, `bcat-topics`, `bcat-data` under `cdn.nintendo.net`)
   serve JSON/plain files over plain TLS. The documentation describes no
   payload signatures, no certificate pinning beyond TLS, and no per-file
   signature scheme. The topics route is the documented
   `/api/nx/v1/titles/<title>/topics` shape. The Wii U-era News NEX
   protocol is not involved.
3. Titles consume BCAT content through the bcat sysmodule, which stores
   what it downloaded into savedata and hands it to the game. Nintendo's
   own data was produced by their internal tools; the consumer-side
   validation a title performs on dataset contents is **title code**, not
   system code — we can only observe it per title (that is the N2 gate).

## Findings

- **TLS is the verification boundary** for the topics/list/data surfaces as
  far as every available source shows. A CFW console with the OpenPak CA
  in its trust store accepts our answers — it already accepted the
  endpoint, certificate and request shape (evidence 1).
- The console retries politely on failure (404 did not break title launch),
  which makes iteration safe: we can serve a candidate shape, watch the
  adapter's `!! UNHANDLED` / access log and the console's behaviour, and
  adjust the single-file wire shapes (`topics.go`, `data.go`).
- Unknown: whether the console applies schema-level validation to topics
  (field names, types, topic id format) and whether specific titles
  hash-check their dataset files. Both are observable, neither is
  documented, and neither can be resolved without hardware — that is the
  spike's residual risk, owned by N1/N2, not by this document.

## Go/no-go per surface

| Surface | Decision | Basis |
| --- | --- | --- |
| News (per-title topics) | **GO** | TLS-only verification per all sources; console already reaches and accepts our endpoint (evidence 1). Wire shape is the spike's adjustable part. |
| Per-title BCAT (list/data) | **GO** | Same TLS boundary; Kinnay documents the routes. Title-side dataset validation is per-title and observed at N2. |
| Console-side patches | **NO-GO** (standing) | PRD §11: the project refuses custom sysmodules. If N1 shows schema-level rejection we iterate shapes; only a sysmodule-level signature check would kill News, and no source shows one. |

## Observation protocol for N1/N2 (owner-run, ~15 min)

1. Start `news` with the seeded article (see README "Seeding a first
   article") and the adapter mounting `render/switchbcat` on the three
   bcat hosts with full request logging.
2. Launch the target title on the console. Capture:
   - adapter log: exact method, path, query (`c[]` values), headers;
   - console: does the News surface show the article?
3. If the article does not render: capture the console's follow-up
   requests (image fetches? retries? alternate route?) and diff the served
   shape against what the console requested. Adjust `topics.go` only.
4. N2: seal a one-file dataset for the title (README "Seeding a dataset"),
   relaunch the title, observe `bcat-list`/`bcat-data` fetches in the
   adapter log and the in-title behaviour.

Output of each step goes into `docs/` as evidence notes, linked from the
capability ledger.
