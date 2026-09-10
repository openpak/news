# Native News probe 0.1.0 hardware result

Date: 2026-09-07. Owner-provided source: `~/report.txt`.

The NRO ran on HOS **22.5.0**, with Atmosphere detected and applet type 0.
The report does not identify the Atmosphere release or emuMMC/sysMMC.

All reported Result codes were `00000000`:

- Initialize `news:p` and `news:v`.
- Read News database version: **3**.
- Read storage usage: **51,544,064 / 104,857,600 bytes**.
- Create database reader and count articles: **102**.
- Invoke `newsDatabaseGetList` for one latest record.

The last call returned success, but the probe did not enter its
`count == 1` branch. Version 0.1.0 omitted the returned count, so this
does not establish whether zero, multiple, or differently measured
results were reported. No article reader was opened, no article was
exported, and no post was submitted.

Version 0.2.0 logs the raw count and whether the single allocated record
has usable, terminated ID fields. It tries an empty filter/order if the
initial query yields no usable record. It never indexes beyond that slot.
An optional X-button export can copy one article via the read API for
private format inspection, bounded to 4 MiB without replacing an existing
sample. Actual schema acceptance and native rendering remain untested.

**Decision:** News service access works on this console. The native
posting/rendering gate remains open pending an actual article test.

## Probe 0.2.0 result

The owner's appended report confirms that both the explicit filter/order
query and the empty-filter/order query return success with **count 0** and
no populated ID. Service checks and total article count are unchanged.
The report contains no X-button export attempt; regardless, that build
had no usable record to export. No sample was provided.

This establishes a mismatch between the list and count paths, not its
cause. Do not infer a corrupt or empty database. Probe 0.3.0 tests a larger
record buffer and both offset values 0 and 1 via the existing public API.
If no record is returned, pressing X uses the documented read-only
`GetNewsDatabaseDump` via `news:a`. It validates the returned size and
SQLite header before exporting private metadata for host-side inspection.
The experiment makes no News-storage or subscription changes.
