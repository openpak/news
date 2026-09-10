# Local News archive inspection and first posting fixture

For the current sample, use the [creation guide](native-news-authoring-guide.md)
and [editable NORMAL article](../examples/native-news/README.md). This document
preserves the investigation history, including unsuccessful LIST/body-array
fixtures. The v0.12 NORMAL/body-map layout was the one confirmed to render.

2026-09-07. Source: the owner's `~/sample.msgpack`, exported
through the read-only News API on HOS 22.5.0 with probe 0.3.0.

All export calls succeeded: initialize viewer, create data service, open
record, get size, read. Export size **862,987 bytes**. The larger list
buffer returned 32 records at offset 0; the smaller buffer previously
returned none. The exact reason for that capacity-dependent behavior is
not yet established. No database snapshot was required.

## Format observations

The archive decodes as a MessagePack map. Format metadata is
`version = {format: 1, semantics: 9}`, language `en-GB`, display type
`LIST`. Identifiers and timestamps are integer values. The body is an
array of section maps with text, image height and binary image fields.
Subject is a map with caption and text. Topic metadata includes name,
publisher, description and an embedded image. Image dimensions observed:

| Field | Encoding | Dimensions |
| --- | --- | --- |
| topic_image | JPEG | 70x70 |
| list_image | JPEG | 358x201 |
| body[0].main_image | JPEG | 800x450 |

The archive also has optional commerce/rating-related sections. No
article text, images, product links, original identifiers, or raw archive
have been copied into this repository. This inspection establishes the
shape of one stored archive, not every accepted shape of PostLocalNews.

## Original experiment

`cmd/newsfixture` generates an original article, **Hello from OpenPak**,
with programmatically drawn blue/green JPEG graphics. It has no sample
input. It embeds into probe 0.4.0 as a generated header. Test ID is
1909260907, topic is `openpak_test`, body is one array section, requested
validity is seven days. NORMAL display type and omission of the sampled
article's commerce/rating fields are explicit hypotheses to test.

On A, the probe verifies fixture freshness and filtered-count behavior,
checks the test ID is absent, calls `newsPostLocalNews` once, logs the
Result and queries the test ID afterward. It never posts automatically
at startup. A successful Result and database row are still distinct from
seeing and opening the article in native News. No single-article removal
method is implemented; the expiry request's actual cleanup behavior is
unknown. No bulk clear or direct system-database mutation is used.

The owner has authorized testing local News delivery. This build makes
the one-article write explicit on screen. The original restrictions on
custom sysmodules, adapter edits and copying third-party code still apply.

## Existing-channel test (0.5.0)

The owner could not find the first article in News and requested using
the same channel as their sample. The sample identifies `nx_news`, named
Nintendo News. Probe 0.5.0 uses that channel ID/name with new test ID
1909260908; the article text and all images remain original OpenPak
content. It does not register a new channel, change subscriptions or
replace the first test record. It retains the filtered duplicate check.

The local report still ended at probe 0.3.0 when this build was prepared.
Thus storage acceptance of 0.4.0 is unconfirmed; attributing its missing
UI card solely to the channel is still a hypothesis. This new build
tests the owner's requested existing-channel route directly.

## Posting blocker confirmed; fixed in 0.6.0

The owner subsequently supplied both posting-run reports. In **both
0.4.0 and 0.5.0**, initialization and the always-false filter check succeed,
but `count test article` returns **0001967D (module 125, description 203)**.
Neither run reaches `PostLocalNews`. The rejected expression is
`CAST(news_id AS INTEGER) = <test-id>`. These reports establish a
preflight failure, not a channel-registration or rendering failure.

Version 0.6.0 preserves the owner's chosen `nx_news` channel and article
1909260908. It removes the CAST query and scans all records using the
already-working list call, comparing zero-normalized ID strings locally.
It refuses to post if the scan is incomplete or malformed. A host mock
test of the actual function passed with address/undefined-behavior
sanitizers. The NRO build passed with warnings treated as errors.
Actual posting remains unverified until the next hardware run.

## First visible card; layout/image experiment (0.7.0)

The owner reports that probe 0.6.0's **Hello from OpenPak** now appears
in native News, but without its banner or article body. This is owner
evidence of a visible native card, not full N1 acceptance. The local report
still ends at 0.5.0 at the time of this follow-up, so stored-byte comparison
has not yet been obtained.

Two concrete differences from the working sample were found: the fixture
combined NORMAL display type with an array-shaped body (the sample uses
LIST), and Go's JPEG encoder produced 4:2:0 images without JFIF headers,
whereas the sample's images are 4:4:4 JFIF. Neither difference is yet a
proven cause. Probe 0.7.0 changes to LIST and uses libjpeg-turbo cjpeg to
encode original images with baseline JFIF/4:4:4. New test ID 1909260909
and title **OpenPak image and body test** distinguish this card.

After posting, the probe locates the new record and reads its bytes back
for comparison against the submitted fixture, logging size and MATCH or
NOT MATCHED. This separates storage transformation from applet rendering
issues. The original user-owned sample remains outside the repository.

## Stored-identity discrepancy and cleanup requirement (0.8.0)

The owner supplied the 0.6.0/0.7.0 reports: both calls to PostLocalNews
return success. The test-ID scan finds zero matching IDs after each post,
despite the owner seeing both titles in News. Consequently the 0.7.0 byte
comparison never ran. Local posting may assign a different stored ID;
this must be established from the stored records rather than assumed.

The first post coincides with a total count change from 102 to 14; the
next adds one (14 to 15). Storage usage also falls. Expiry cleanup is a
possible explanation, not a confirmed cause. The probe made no explicit
clear-storage or subscription-reset calls.

The owner explicitly requests deletion of test articles to avoid filling
News with tests. Cleanup is now part of the required workflow. Further
posting is disabled in 0.8.0: A performs read-only enumeration and content
inspection. It exports candidates containing the two known test titles,
logs their stored News IDs and tries a private database snapshot for
targeted cleanup research. A title match is a candidate, not authorization
to delete an arbitrary article; verify decoded content and metadata first.
No deletion has been implemented or performed yet. Any future cleanup
must target only verified OpenPak test records, not clear all News.

## Exact stored payloads and compact-format/expiry test (0.9.0)

The owner supplied both candidate archives, the report and the database
snapshot. Stored IDs are `LS00000000001909260908` and
`LS00000000001909260909`, with empty user IDs. Their SHA-256 hashes match
the original fixtures exactly. In particular the 0.7.0 stored file is a
byte-for-byte match: News did not strip its body or images. Both database
rows have `opted_in=1`. Missing rendering is not explained by data loss
or those opt-in flags.

The fixture encoder used int64 encodings even for small positive values,
and map32/str32 encodings for small containers/strings. These are valid
MessagePack, but differ from the compact encodings in the working sample.
The 0.9.0 encoder emits compact widths, including positive fixints and
uint32 timestamps/IDs, and was checked byte-for-byte against an independent
MessagePack encoder. Compatibility with the applet remains a hypothesis.

The owner explicitly authorized removing test articles. Probe 0.9.0's A
action checks each exact stored ID and empty user ID, then verifies the
payload against the known original SHA-256 or exact current-build test
bytes. It refuses unknown or changed content. It submits past-expiry
replacements for the same two IDs and re-enumerates to check whether they
disappear. **This is an expiry-cleanup experiment, not a verified deletion
API.** If rows remain, the tool reports removal unconfirmed. It never
calls ClearStorage, resets subscriptions or directly modifies SQLite.
The original test archives are preserved in the owner's home directory.

B reuses numeric ID 1909260909 (stored LS-prefixed ID above) for
**OpenPak compact-format test**, replacing only verified test content or
creating it if absent. It does not allocate another test ID. The same
channel and original graphics are retained to isolate serialization.
Host sanitizer checks exercise exact-payload authorization, cross-ID
rejection and rejection of modified bytes. Full Go vet/race tests pass.

## Cleanup visually confirmed; compact post not located (0.10.0)

The owner reports both test cards disappeared. Their subsequent 0.9.0
report starts with 13 records (the prior baseline without the two tests).
It contains a B action: PostLocalNews succeeds but the expected LS-prefixed
ID remains absent. The cleanup A action itself is not present in this
report, so its intermediate results are unavailable. Do not claim the new
compact article was successfully stored solely from IPC success.

Raw tag inspection confirms the original sample uses positive fixints and
uint32 IDs/timestamps, matching the compact encoder. Signedness alone is
not established as the cause of the missing record. The sampled article
was downloaded content; built-in local records exist in the snapshot,
including `LS00000000000000010000` under `nx_notice`.

Probe 0.10.0 is read-only again. A logs the complete bounded record-ID
inventory and exports that specific built-in local record to
`local-reference.msgpack`, plus any matched test articles to distinct
`stored-v010-*.msgpack` files and a `news-database-v010.sqlite` snapshot.
This allows comparison against an actual local News reference and detects
unexpected identities assigned after the compact post, without more writes.

## Built-in local schema comparison and test (0.11.0)

The supplied 0.10.0 report and 4096-byte SQLite snapshot contain exactly
13 baseline records, with neither prior OpenPak ID present and no unexpected
new local ID. All ten LS records are built-in nx_notice entries. The compact
post was therefore not found under a different ID in this snapshot.

The exported local reference is 524288 bytes: its first MessagePack object
ends at byte 279126, followed only by zero padding. Its root is a 20-field
map beginning with version {format: 1, semantics: 1}; display_type is NORMAL
and body is a map, not an array. Text contains no HTML tags. The list image
is 358x201 and main image 800x364, both RGB JPEG with 4:4:4 sampling. In
contrast, the downloaded sample uses semantics 9, LIST, and an array body.
This establishes different observed schemas, not a universal restriction
on what PostLocalNews or the applet accepts. Padding could reflect the
built-in resource allocation; this test does not assume it is required.

Probe 0.11.0 follows the local layout with ordered maps (version first),
semantics 1, NORMAL and a single body map. It uses original text/JFIF images,
adds a future expiry, omits the reference's system-applet action, and keeps
the owner's nx_news topic. This changes several schema details together;
a successful result will not isolate a single causal field or map order.
No private reference text or image data was copied into the repository.

B reuses 1909260909, checks the full bounded inventory first, and either
requires absence or exact current-build live/expired bytes with empty
user ID. It skips an already-stored identical live payload. After posting,
it re-enumerates and reads the actual bytes before claiming storage success,
then exports candidates and news-database-v011.sqlite for inspection.
A expires only this same verified record and checks absence afterward.
No new test IDs, bulk clearing, subscriptions or direct SQLite writes.
Unknown payloads fail closed; use this same build for cleanup before updating.
The new C guard passed host sanitizer checks and the NRO builds with warnings
as errors. Independent MessagePack decoding and image dimension checks pass.
Native body/image rendering and the new build's cleanup await hardware.

## Local schema still absent; fresh-ID control (0.12.0)

The supplied 0.11.0 report includes two A presses while the target was
absent, followed by B in a later run. B called PostLocalNews successfully,
but readback and the full snapshot still contained only 13 baseline records.
No new test article was found. The existing local-reference export was
preserved (errno 17), as designed. There is no evidence of renderability
from this test because its payload was not stored.

Both 0.9.0 and 0.11.0 reused the previously expired ID 1909260909. A retained
identity/history entry suppressing reinsertion is an unproven hypothesis.
Probe 0.12.0 controls for identity by changing only news_id to 1909260910;
both live and cleanup MessagePack payloads otherwise reproduce their 0.11.0
hashes exactly when the old ID is restored. The same title, timestamps,
channel, schema, key order and graphics remain. This is one bounded new test
ID, not a runtime ID allocator. Repeated B presses reuse it; A verifies exact
payload bytes before submitting expiry cleanup for that ID only. Previous
IDs remain absent in the supplied database and are not targeted by this build.

The NRO and Go fixture tests pass. Hardware storage and cleanup of the new
ID are pending. If stored, body/image rendering will still need observation.

## Native rendering confirmed by owner (0.12.0)

The owner supplied the report, pre-cleanup database snapshot and stored
article, then confirmed "it worked!" after the requested News rendering test.
B increased the record count from 13 to 14. Stored local ID is
LS00000000001909260910, topic nx_news, with opted_in=1. The 25142-byte export
matches the generated live fixture exactly (SHA-256
2ab40068e2de42852d22d87f1fa9f2ca8e6a76d8c05e328ecabaa32604f87ab9).
This establishes an owner-observed working local article rendering path.
It does not establish network BCAT delivery or subscription refresh.

Only news_id changed from 0.11.0, strongly supporting an identity-related
reinsertion obstacle for the previously expired ID. The mechanism and any
retention duration are unknown. The successful fixture combines semantics 1,
NORMAL, ordered maps and an object-shaped body; which differences were
necessary relative to the earlier title-only fixture remains unisolated.

A subsequently replaced the live bytes with the exact expired fixture,
verified by readback. The immediate count remained 14 and the target remained
present: cleanup reported UNCONFIRMED. This is confirmed expiry replacement,
not confirmed immediate deletion or storage reclamation. The supplied SQLite
snapshot predates A and cannot establish the post-cleanup database state.
The same build can retry A for its verified expired payload without allocating
another ID; a subsequent report is needed to determine whether the record is
removed. Do not describe expiry cleanup as a reliable immediate delete API.
