# Evidence: news

**Patched-console experiment (2026-09-09):** the owner has authorized and
installed a build-specific OpenPak BCAT verification-key patch. An isolated
remote CDN now serves original signed/encrypted catalog fixtures for testing
with probe v0.19 ZL. After a separate News edge-token omission patch and reboot,
native retrieval succeeds and reports 155 decoded catalog bytes. Exported
plaintext comparison, Find Channels parsing and subscriptions remain pending.
The UI has requested OpenPak catalog/icon/detail. Server v004 also serves an
empty online_archives container; the channel-page retest is pending.
See [experiment and rollback](native-news-container.md).
The earlier server-only NO-GO below still applies to unpatched consoles.

**Console finding (NB-00, see `docs/spike-nb00.md` +
`docs/spike-nb00-review.md`):** the Switch's topics/list/data payloads are
signed, optionally encrypted **bcat content containers** — RSA-2048 with a
Nintendo key held in the sysmodule. Without Nintendo's private key (and
refusing console-side patches), console News and console BCAT are **NO-GO**.
At the time of that server-only spike, this repo served no bcat-hostname console routes; contract
request NB-1 is withdrawn. What reaches consoles is dataset placement on
the SD by the setup homebrew (LayeredFS), driven by this repo's delivery
contract.

**Native News follow-up (2026-09-07):** a separate local IPC route,
`newsPostLocalNews`, is now under investigation for a foreground homebrew
helper. The later v0.12 test demonstrated local article rendering; subscribable
channels and automatic delivery remain unverified. See
[the evidence and hardware experiment](native-news-reopen.md).

## N1' gate

Gate: Citron, pointed at the emulator API, consumes the dataset (article
+ data). Emulators take the BCAT passphrase from the game and verify no
container signature — this is the path that works.
