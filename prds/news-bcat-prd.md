# OpenPak News PRD (platform-neutral content, per-console delivery) and Switch BCAT

> **Current Switch client (2026-10-13): Harbor**, at
> `~/REPOS/Openpak/emulators/harbor`. Harbor is an independent
> OpenPak project based on Eden. Citron, Ryujinx (desktop and Android), and
> the former OpenPak Eden checkouts have been removed. References to those
> forks below record historical evidence only: do not run their commands,
> reinstall them, or select them for current work. Use Harbor's README and
> current handoff for builds and testing; old results do not establish Harbor parity.

Latest status (2026-09-09, supersedes experiment gates in the historical updates
below): v0.19 on the owner's patched HOS 22.5.0 console successfully downloaded
and decoded the signed catalog (155 bytes). Both the retail-key patch and a
separate News memory-download edge-token omission patch are installed. The UI
subsequently requested catalog/icon/detail. Server v004 now serves an empty
online_archives response based on the firmware parser; channel-page acceptance
after that fix, subscriptions and automatic article delivery are still pending.
N1 and production NB-1 remain incomplete. No custom sysmodule was introduced.

Agent entry points: [creation guide](../docs/native-news-authoring-guide.md),
[editable channel/article sample](../examples/native-news/README.md),
[handoff task](../docs/native-news-agent-handoff.md), and
[hardware/deployment evidence](../docs/native-news-container.md).

Status: draft for agent assignment (2026-09-07). Starts with a research spike because the
console's signature checks decide the whole design.

Implementation status (2026-09-08): the server-only console delivery spike
was corrected to NO-GO in `news/docs/spike-nb00-review.md`; NB-1 was
withdrawn. The owner has reopened native News investigation for a modded
Switch. `news/docs/native-news-reopen.md` records a distinct local IPC
path (`PostLocalNews`): probe 0.12.0 now stores an original article and the
owner confirms native rendering. Find Channels, subscription and server
delivery remain separate unverified gates, so N1 is not complete. Expiry
replacement works but immediate deletion is unconfirmed. This does not
change the no-custom-sysmodule constraint below.

Implementation update (2026-09-09): the owner explicitly authorized a console
patch and NRO installation, superseding the previous no-console-patch scope
for this experiment. A build-pinned BCAT retail public-key replacement and
probe v0.15 installer now exist; see `news/tools/news-patch/README.md`.
This is an ExeFS data patch, not a custom sysmodule. It affects game BCAT as
well as News. Host validation passed; hardware application, native container
acceptance, catalog schema compatibility and subscriptions are unverified.
The proven local article format is retained.

Server experiment update (2026-09-09): following the owner's instruction to
implement and test signed server responses, an isolated fixture CDN now serves
an OpenPak-signed, encrypted catalog/detail/icon on the remote News topics SNI.
Other paths forward to the existing adapter; nx-baas source is unchanged.
This limited experiment supersedes the earlier hostname ownership restriction
for that one route. The historical News AES profile remains a candidate until
native decoding succeeds. See `news/docs/native-news-container.md` for evidence,
deployment and rollback. Catalog rendering, subscriptions and automatic article
delivery are not yet confirmed; N1 and production NB-1 remain incomplete.

## 1. Product objective

One news service for the whole network: an article is written once ("Stardew Valley server
now live on PS4, Switch and Xbox"), targeted by platform, title, region and language, and
delivered through every channel a player has. Consoles with a native channel get it there
(Switch News via BCAT topics; Wii U/3DS via SpotPass/BOSS later); every player also sees it
on the website, in the companion apps, and in-game where a title server can show a message
of the day. Separately, Switch titles receive their BCAT data (schedules, seasonal content)
from OpenPak instead of a dead Nintendo endpoint. Both are new.

Design rule: content, targeting and authoring are platform-neutral; each console channel is
a renderer package. Per-title BCAT data is title data, Switch-only by nature, and lives with
the Switch renderer.

## 2. Relationship to existing plans

The bcat hosts (`bcat-topics-lp1`, `bcat-list-lp1`, `bcat-data-lp1` under `cdn.nintendo.net`)
are console-facing and routed by the Switch adapter. This PRD owns the service; the adapter
agent wires the hostnames (Contract request NB-1). Do not edit nx-baas.

## 3. Evidence and current baseline

- Console today (evidence): `GET bcat-topics-lp1.cdn.nintendo.net/api/nx/v1/titles/<title>/
  topics?c[]=FR&c[]=GB` on title launch (seen for Outbound and a system title); we 404.
  A hosts file can redirect all three bcat hosts (hosts-file evidence only).
- Public documentation: Kinnay wiki BCAT-Servers (bcat-list, bcat-topics, the topics route).
  The News NEX protocol page is Wii U-era and not this.
- Earlier research notes (evidence only): cached per-title BCAT files can be served to emulators and
  copied to LayeredFS on console; that route does not serve News.

## 4. Users and journeys

1. Console player: News shows the OpenPak channel; articles with images render.
2. Title launch: BCAT data for a supported title downloads; the title shows current
   schedules and events.
3. Operator: writes an article on the website (title, body, image, publish date, audience by
   country and language); it appears on consoles within the console's refresh cycle.
4. Title maintainer: uploads a BCAT dataset for a title with a validity window.

## 5. Scope and non-goals

In: platform-neutral article model with targeting (platforms, titles, regions, languages),
authoring pages, a read API for the website, companion apps and title servers (message of
the day); the Switch renderer (topics, list and data endpoints in the console's format);
per-title Switch BCAT datasets; emulator delivery of BCAT data. Planned, not in this PRD's
milestones: Wii U/3DS SpotPass renderer (with `nn-boss`), PlayStation/Xbox in-game surfaces
via their title servers. Out: Nintendo's own feeds, third-party feeds, push notifications
for news (the apps' push is for social events only).

## 6. Independent implementation rules (apply verbatim)

- OpenPak servers are AGPL-3.0-only and OpenPak-authored.
- Go 1.26, module `github.com/openpak/<repo>`, AGPL-3.0-only, `LICENSE` verbatim from
  gnu.org. Server-rendered templates and minimal browser JavaScript for web surfaces.
  Native mobile code is the recorded non-Go exception.
- Talk to the account core **only** through its versioned contracts (gRPC: Identity, Links,
  Social, Sessions, Invitations, Events; HTTP `/v1/*`). Never read its database. If a
  contract is missing, write the proposal in this PRD's "Contract requests" section and
  raise it as an `account` issue; do not fork the core.
- Do not touch `Openpak/nx-baas` (the Switch adapter, owned by the T4/T5 agent) or the
  console-facing hostnames it serves. Console-side needs go into "Contract requests".
- No secrets, tokens, saves, firmware, ROMs, game code or asset dumps in any
  repository. **Rectified 2026-09-11 (owner decision):** gameplay
  screenshots captured by OpenPak operators from legitimate play are
  permitted in the `news` repository as editorial imagery for News articles,
  under these terms: only what an article needs (sized to article limits,
  no full-resolution distribution), the game credited in the article text,
  and any image removed on the rights holder's request. This exception
  covers News editorial imagery only; game-server repositories keep the
  strict clean-room rule, and nothing here authorises distribution of
  ROMs, firmware, keys, saves or extracted game assets. `example.env` only.
- Definition of done for every milestone: `go vet ./...` and `go test -race ./...` green in
  CI, a runbook in `README.md`, and the acceptance journey below reproduced by the owner.

## 7. Functional requirements

- NB-00 Research spike (blocking): determine what the console verifies on News and BCAT
  payloads (signatures, certificates, hashes) and what a CFW console with the OpenPak CA
  accepts. Method: serve one topic and one article through the adapter's routes and read the
  console's behaviour and the adapter's debug log. Output: written finding and a go/no-go per
  surface (News, per-title BCAT).
- NB-01 Topics API: per-title topics with the country and language filters the console sends.
- NB-02 List and data APIs: article and data file listing with hashes, sizes, validity
  windows, and the download format the console expects (from NB-00).
- NB-03 Content model: article (id, title, body, images, locales, publish and expire,
  audience = platforms + titles + regions + languages, "all" allowed for each); BCAT dataset
  (title id, files, validity). One article can target every platform at once.
- NB-07 Read API (platform-neutral): list and fetch articles for an audience, with `ETag`;
  consumed by the website, the companion apps and title servers. This API is the source
  every renderer reads from; renderers never hold content of their own.
- NB-08 Renderer contract: a Go interface (audience in, console-format out) with the Switch
  BCAT renderer as the first implementation; the SpotPass renderer implements the same
  interface later without touching content code.
- NB-04 Authoring: role-gated website pages for articles and datasets; preview.
- NB-09 Channel and lifecycle management API (added 2026-09-16): extend the authoring
  gRPC beyond articles and datasets so operators manage the whole catalogue through the
  API — create, update, retire and delete channels (topic id, lineup order, per-channel
  art and language set) and delete articles (today only unpublish exists). Today channels
  exist only as static seed files (`channels/*.json` compiled by `newsseed`), so any
  lineup change is a rebuild and resign; the management API must let the renderers pick
  channel and article changes up from the store without a rebuilt seed.
- NB-05 Emulator delivery: same data over the emulator API so Citron can place BCAT files in
  its LayeredFS folder.
- NB-06 Caching headers for a CDN-shaped host: `ETag`, `Last-Modified`.

## 8. Architecture and ownership

Repo `news`: `cmd/news`, Postgres for content, object store for images and datasets;
packages: `content` (model, targeting, read API, authoring gRPC), `render/switchbcat`
(console-facing handlers the Switch adapter mounts on the bcat hostnames, plus per-title
BCAT datasets), later `render/boss` for Wii U/3DS. The website's authoring pages and the
apps' news screens consume `content` only.

## 9. Contract requests

- NB-1 Adapter mounts the `news` console handlers on the three bcat hostnames and adds them
  to its certificate SAN list (already in the console's redirect list).
- NB-2 Website authoring pages consume the `news` gRPC.

## 10. Milestones and acceptance

- N0 Spike finding written (NB-00). Gate: owner accepts the go/no-go.
- N1 Topics, list and one article rendered on a real console. Gate: article visible in News.
- N2 Per-title BCAT for one title. Gate: the title consumes the dataset.
- N3 Authoring pages, read API, emulator delivery. Gate: an article written on the site
  targeted at "all platforms" reaches the Switch News channel, the website and a companion
  app from one record; Citron receives the dataset.
- N4 (when the first Wii U/3DS title lands) SpotPass renderer on the same content. Gate: the
  same article appears as a 3DS notification without any content change.

## 11. Risks

- Signature verification may make console News impossible without console-side patches the
  project refuses (no custom sysmodule). N0 decides; per-title BCAT via LayeredFS stays
  possible either way.
- Only operators publish; no user-generated news.

## 12. Evidence references

Console log lines for `bcat-topics-lp1` (2026-09-07); hosts-file redirect evidence;
Kinnay BCAT-Servers page.
