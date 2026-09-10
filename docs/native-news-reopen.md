# Native Switch News: reopened investigation

Current entry point: [creation guide](native-news-authoring-guide.md) and
[agent handoff](native-news-agent-handoff.md). On 2026-09-09, patched native
catalog retrieval and decoding succeeded with v0.19. Server v004 subsequently
added the archive route; channel-page acceptance, subscriptions and automatic
article delivery remain pending. The local-only status below is historical.

Date: 2026-09-08. Status: local posting and native article rendering are
confirmed on HOS 22.5.0 with probe 0.12.0. Network delivery and native
channel discovery/subscription remain unverified.
See [the local format and posting experiment](native-news-local-format.md).

The owner has asked to pursue articles in the native News applet on a
modded Switch. The existing server-only BCAT finding remains in force.
It does not establish that every local route into the News applet is
impossible. Do not mark N1 complete or restore the removed console routes
on the strength of this document.

## Evidence and limits

- [BCAT container documentation](https://switchbrew.org/wiki/BCAT_Content_Container)
  describes the RSA signature and encryption header. An OpenPak TLS CA
  does not replace that application-level trust mechanism.
- [News service documentation](https://switchbrew.org/wiki/BCAT_services#PostLocalNews)
  documents `INewsService` command 10100, `PostLocalNews`, accepting
  MessagePack. This is a separate IPC entry point from the encrypted
  download command 30500. Its acceptance rules need testing.
- [libnx public header](https://github.com/switchbrew/libnx/blob/master/nx/include/switch/services/news.h)
  exposes `newsInitialize(NewsServiceType_Post)`, `newsPostLocalNews`,
  and database/data readers. Its
  [implementation](https://github.com/switchbrew/libnx/blob/master/nx/source/services/news.c)
  dispatches the post buffer directly to command 10100. This proves a
  client API exists, not that arbitrary articles pass the firmware parser.

Hardware confirms that a foreground homebrew helper can post an original
local article that the native applet displays, without a replacement
sysmodule. The probe embeds that article; fetching it from OpenPak over
the network remains to be implemented and tested. A visible local article, a channel listed
in Find Channels, and unattended network refresh are separate acceptance
criteria. Local posting alone establishes neither of the latter two.

## First hardware experiment

1. Record firmware, Atmosphere version, emuMMC/sysMMC, homebrew launch
   mode, console address, and available transfer method. Keep device
   identifiers and any private data outside the repository.
2. Build a small diagnostic NRO against a recorded libnx revision. First
   test service initialization and read-only metadata access; log operation
   names and full Result codes. Do not clear storage or subscriptions.
3. Establish the accepted local MessagePack structure for that firmware.
   Inspect local data only if needed; do not commit an archive, database,
   Nintendo article text, or images. Document field types independently.
   Database column names alone are not an article schema.
4. Prepare one original OpenPak article and original image. Validate size,
   encoding, locale and timestamps on the host before submitting it.
   Agree a test ID and removal/recovery procedure before writing persistent
   News data; do not overwrite an existing article or edit the system
   database directly.
5. Submit once through `newsPostLocalNews`, record its Result, then open
   native News. Verify the card, title, body and image, and persistence
   after reopening. An IPC success code by itself is not acceptance.
6. Separately test channel identity/subscription behavior. Report whether
   the result is a local article only or a real subscribable channel.

The diagnostic and posting steps require execution on the owner's Switch.
The [News probe](../tools/news-probe/README.md) was built with devkitPro in
Docker; its pinned toolchain is recorded in `../tools/news-probe/BUILD.md`.
Probe 0.12.0 successfully stored an original article byte-for-byte and the
owner confirmed native rendering. Expiry replacement also succeeds, but
immediate record removal is not confirmed. Reusing an expired test ID
failed to store a new article; changing only its ID succeeded.

## Integration after local posting succeeds

Keep authoring and audience selection in the existing Go content service.
Add a separate local-News renderer only after the schema is verified;
`render/switchbcat` currently describes dataset delivery, not News articles.
The helper fetches published content for the console's locale/audience,
checks the response and assets, and posts articles through IPC. Track
article IDs and revisions to prevent duplicates. Work out expiry and
withdrawal behavior before allowing repeated production syncs.

A foreground helper requires the player to run it to refresh. Automatic
background delivery is another design decision; the PRD currently excludes
custom sysmodules and console-side patches. Do not silently relax that
constraint or claim that a foreground test meets the refresh-cycle journey.
The owner's follow-up request to test the proposed helper authorizes the
small C/libnx diagnostic as native tooling; the server and content model
remain Go. It does not authorize a custom sysmodule or change the standing
patch constraint.

## Decisions and contracts

- Server-only native BCAT: existing NO-GO remains.
- Native News through local IPC: original article posting/rendering confirmed;
  network sync and reliable withdrawal remain work items.
- Subscribable OpenPak channel: unverified, independently gated.
- NB-1: remains withdrawn pending evidence for an actual CDN delivery path.
  This investigation does not require changes to `nx-baas`.
- Foreground diagnostic: owner requested hardware testing; NRO built.
- HOS 22.5.0 and Atmosphere presence are recorded by the successful probe.
  Native channel discovery/subscription is the owner's next requested test.
  Console-side patches have not been authorized.

Per-title emulator dataset delivery remains separate. The previous docs'
generic LayeredFS placement claim also needs title-specific validation:
an arbitrary SD directory is not evidence that a title's BCAT storage API
will consume it. That work cannot satisfy the native News gate.

## Next acceptance gate: Find Channels

The owner requests native channel discovery, subscription and delivery from
OpenPak. Capture three actions separately: open Find Channels, open a channel,
and follow/unfollow it. Do not assume that subscription changes make an HTTP
request; they may be local state followed by a later feed fetch. Correlate
request times with owner actions and, if necessary, before/after local state.

The prior catch-all captured GET /api/nx/v1/topics/catalog with l=en-GB and
c[]=GB twice. Related documented routes are /api/nx/v1/topics/<name>/detail,
/api/nx/v1/topics/<name>/icon, /api/nx/v2/latest_news/<name>, and
/api/nx/v2/topics/<name>/online_archives. Their exact methods, formats and
verification rules must be established rather than inferred from names.
See the [BCAT endpoint research](https://github.com/kinnay/NintendoClients/wiki/BCAT-Servers).
That source explicitly documents a BCAT container for per-title topics;
it does not document the catalog response schema. A catalog 404 does not
prove either catalog acceptance or rejection of a candidate format.

Record only timestamps, method, allowlisted News host/path, language/country,
status and response size. Omit authentication headers, serials and device or
account identifiers. Existing Nintendo response bodies, if supplied for
format inspection, remain private and outside repositories.

As of this follow-up, /tmp/nx-baas.log is absent and the previous local
21000/21999 listeners are absent. The active catch-all location/log source
is needed before a live watch can start. No routing or adapter modifications
have been made. After observing the wire contract, prepare the catalog
experiment in news and an adapter contract request; do not invent JSON
handlers or equate local IPC success with BCAT download verification.
