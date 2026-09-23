// SPDX-License-Identifier: AGPL-3.0-only
#include <switch.h>
#include "test_article.h"
#include "cleanup_articles.h"
#include "news_patch.h"
#include "news_auth_patch.h"
#include <errno.h>
#include <malloc.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/stat.h>
#include <time.h>

static FILE *report;
static NewsRecord selected_record;
static bool have_record;

static void log_line(const char *fmt, ...) {
    va_list args;
    va_start(args, fmt);
    vprintf(fmt, args);
    va_end(args);
    if (report) {
        va_start(args, fmt);
        vfprintf(report, fmt, args);
        va_end(args);
        fflush(report);
    }
    consoleUpdate(NULL);
}

static bool result(const char *operation, Result rc) {
    log_line("%-29s %08lX (%lu-%lu)\n", operation,
        (unsigned long)rc, (unsigned long)R_MODULE(rc),
        (unsigned long)R_DESCRIPTION(rc));
    return R_SUCCEEDED(rc);
}

#include "transport.h"

static void manage_patch(bool remove_patch, bool auth_patch) {
    const unsigned char *payload = auth_patch ? news_auth_patch : news_patch;
    const size_t payload_size = auth_patch ? sizeof(news_auth_patch) : sizeof(news_patch);
    const char *path = auth_patch ? NEWS_AUTH_PATCH_PATH : NEWS_PATCH_PATH;
    const char *directory = auth_patch ? NEWS_AUTH_PATCH_DIR : NEWS_PATCH_DIR;
    log_line("Selected patch: %s\n", auth_patch ? "News token omission" : "BCAT verification key");
    // Removal remains available after a firmware update.
    if (!remove_patch) {
        if (!hosversionIsAtmosphere()) {
            log_line("Patch requires Atmosphere.\n");
            return;
        }
        u64 pid = 0;
        if (!result("pm:dmnt initialize", pmdmntInitialize())) return;
        Result rc = pmdmntGetProcessId(&pid, 0x010000000000000cULL);
        pmdmntExit();
        if (!result("BCAT process", rc)) return;
        if (!result("ldr:dmnt initialize", ldrDmntInitialize())) return;
        LoaderModuleInfo modules[16] = {0};
        s32 count = 0;
        rc = ldrDmntGetProcessModuleInfo(pid, modules, 16, &count);
        ldrDmntExit();
        if (!result("BCAT module build", rc)) return;
        bool supported = false;
        for (s32 i = 0; i < count && i < 16; ++i)
            if (!memcmp(modules[i].build_id, news_patch_build, 32)) supported = true;
        if (!supported) {
            log_line("Unsupported BCAT build; no patch installed.\n");
            return;
        }
    }
    FILE *existing = fopen(path, "rb");
    if (existing) {
        unsigned char bytes[sizeof(news_patch) + 1];
        size_t size = fread(bytes, 1, sizeof(bytes), existing);
        bool same = !ferror(existing) && size == payload_size &&
            !memcmp(bytes, payload, payload_size);
        fclose(existing);
        if (!same) {
            log_line("Existing patch differs; refusing to replace or remove it.\n");
            return;
        }
        if (remove_patch) {
            if (unlink(path)) log_line("Patch removal failed: %d\n", errno);
            else log_line("Selected patch removed. Reboot Atmosphere to restore original behavior.\n");
        } else log_line("Matching patch already installed. Reboot to apply it.\n");
        return;
    }
    if (errno != ENOENT) {
        log_line("Cannot inspect patch path: %d\n", errno);
        return;
    }
    if (remove_patch) { log_line("Patch is not installed.\n"); return; }
    const char *dirs[] = {"sdmc:/atmosphere", "sdmc:/atmosphere/exefs_patches", directory};
    for (size_t i = 0; i < sizeof(dirs)/sizeof(dirs[0]); ++i) {
        if (mkdir(dirs[i], 0777) && errno != EEXIST) {
            log_line("Cannot create patch directory: %d\n", errno);
            return;
        }
    }
    const char *temporary = auth_patch ? NEWS_AUTH_PATCH_PATH ".tmp" : NEWS_PATCH_PATH ".tmp";
    int fd = open(temporary, O_WRONLY | O_CREAT | O_EXCL, 0666);
    if (fd < 0) { log_line("Cannot create patch temporary file: %d\n", errno); return; }
    bool ok = write(fd, payload, payload_size) == (ssize_t)payload_size;
    if (ok && fsync(fd)) ok = false;
    if (close(fd)) ok = false;
    if (ok && rename(temporary, path)) ok = false;
    if (!ok) {
        unlink(temporary);
        log_line("Patch installation failed.\n");
        return;
    }
    log_line("Selected OpenPak patch installed. Reboot Atmosphere to apply.\n");
    if (auth_patch) {
        log_line("News memory downloads omit the edge token for every URL while installed.\n");
        log_line("Use OpenPak DNS. D-pad Left removes this separate patch.\n");
    } else {
        log_line("Affects all retail BCAT verification on this build, including games.\n");
        log_line("L removes the verification-key patch.\n");
    }
    log_line("Applies to matching Atmosphere boots sharing this SD.\n");
}

static void probe(void) {
    const u32 version = hosversionGet();
    log_line("OpenPak News probe 0.22.0 -- firmware reception (subscription status 2)\n");
    log_line("HOS: %u.%u.%u  Atmosphere detected: %s\n",
        HOSVER_MAJOR(version), HOSVER_MINOR(version), HOSVER_MICRO(version),
        hosversionIsAtmosphere() ? "yes" : "no");
    log_line("Applet type: %u  UTC epoch: %lld\n\n",
        (unsigned)appletGetAppletType(), (long long)time(NULL));

    // Opening the post service does not submit an article.
    if (result("news:p initialize", newsInitialize(NewsServiceType_Post))) {
        log_line("Post session opened; no post sent.\n");
        newsExit();
    }

    if (!result("news:v initialize", newsInitialize(NewsServiceType_Viewer)))
        return;

    u32 db_version = 0;
    if (result("database version", newsGetDatabaseVersion(&db_version)))
        log_line("Database version: %lu\n", (unsigned long)db_version);

    u64 used = 0, total = 0;
    if (result("savedata usage", newsGetSavedataUsage(&used, &total)))
        log_line("News storage: %llu / %llu bytes\n",
            (unsigned long long)used, (unsigned long long)total);

    NewsDatabaseService db = {0};
    if (result("create database reader", newsCreateNewsDatabaseService(&db))) {
        u32 count = 0;
        if (result("count records", newsDatabaseCount(&db, "1", &count)))
            log_line("News records: %lu\n", (unsigned long)count);

        const char *filters[] = {"1", ""};
        const char *orders[] = {"received_at DESC", ""};
        for (unsigned attempt = 0; attempt < 4 && !have_record; attempt++) {
            NewsRecord records[32] = {0};
            count = 0;
            log_line("List attempt %u (capacity 32, offset %u):\n", attempt + 1, attempt / 2);
            Result rc = newsDatabaseGetList(&db, records, 32,
                filters[attempt % 2], orders[attempt % 2], &count, attempt / 2);
            if (!result("read record metadata", rc))
                continue;
            NewsRecord record = records[0];
            const bool valid = record.news_id[0] &&
                memchr(record.news_id, 0, sizeof(record.news_id)) &&
                memchr(record.user_id, 0, sizeof(record.user_id));
            log_line("Returned count: %lu; usable ID fields: %s\n",
                (unsigned long)count, valid ? "yes" : "no");
            // Inspect only the first slot; do not index by an unverified count.
            if (count && valid) {
                selected_record = record;
                have_record = true;
            }
        }
        newsDatabaseClose(&db);
    }
    newsExit();
}

static void export_database(void) {
    log_line("Reading private database snapshot (no News writes).\n");
    if (!result("snapshot: news:a initialize", newsInitialize(NewsServiceType_Administrator)))
        return;
    const size_t capacity = 16 * 1024 * 1024;
    unsigned char *buffer = calloc(1, capacity);
    if (!buffer) {
        log_line("Cannot allocate snapshot buffer.\n");
        newsExit();
        return;
    }
    u64 used = 0;
    Result rc = newsGetNewsDatabaseDump(buffer, capacity, &used);
    newsExit();
    log_line("Snapshot returned size: %llu\n", (unsigned long long)used);
    if (!result("read database snapshot", rc) || used < 100 || used > capacity ||
        memcmp(buffer, "SQLite format 3\0", 16) != 0) {
        log_line("No validated SQLite snapshot; nothing exported.\n");
        free(buffer);
        return;
    }
    const char *path = "sdmc:/switch/openpak-news-probe/news-database-v012.sqlite";
    int fd = open(path, O_WRONLY | O_CREAT | O_EXCL, 0600);
    if (fd < 0) {
        log_line("Cannot create database file (errno %d); existing file preserved.\n", errno);
        free(buffer);
        return;
    }
    FILE *out = fdopen(fd, "wb");
    if (!out) {
        close(fd);
        unlink(path);
        free(buffer);
        log_line("Cannot create snapshot stream.\n");
        return;
    }
    bool ok = fwrite(buffer, 1, (size_t)used, out) == used;
    if (fclose(out) != 0)
        ok = false;
    free(buffer);
    if (ok)
        log_line("Saved news-database-v012.sqlite. Keep outside repositories.\n");
    else {
        unlink(path);
        log_line("Snapshot write failed; partial file removed.\n");
    }
}

static bool contains_text(const unsigned char *data, size_t size, const char *text) {
    size_t n = strlen(text);
    if (size < n)
        return false;
    for (size_t i = 0; i <= size - n; i++)
        if (memcmp(data + i, text, n) == 0)
            return true;
    return false;
}

static bool save_candidate(const unsigned char *data, size_t size, unsigned index, bool reference) {
    char path[160];
    if (reference)
        snprintf(path, sizeof(path), "sdmc:/switch/openpak-news-probe/local-reference.msgpack");
    else
        snprintf(path, sizeof(path), "sdmc:/switch/openpak-news-probe/stored-v012-%02u.msgpack", index);
    int fd = open(path, O_WRONLY | O_CREAT | O_EXCL, 0600);
    if (fd < 0) {
        log_line("Cannot create export %s (errno %d).\n", path, errno);
        return false;
    }
    FILE *out = fdopen(fd, "wb");
    if (!out) {
        close(fd);
        unlink(path);
        return false;
    }
    bool ok = fwrite(data, 1, size, out) == size;
    if (fclose(out) != 0)
        ok = false;
    if (!ok)
        unlink(path);
    else
        log_line("Saved %s (%u bytes).\n", path, (unsigned)size);
    return ok;
}

static void inspect_stored_articles(void) {
    bool snapshot_needed = false;
    log_line("Scanning stored articles by title. No posting or News writes.\n");
    if (!result("inspect: news:v initialize", newsInitialize(NewsServiceType_Viewer)))
        return;
    NewsDatabaseService db = {0};
    if (!result("inspect: create database", newsCreateNewsDatabaseService(&db))) {
        newsExit();
        return;
    }
    u32 total = 0;
    if (!result("inspect: count records", newsDatabaseCount(&db, "1", &total)))
        goto done;
    if (total > 512) {
        log_line("Record count exceeds bounded diagnostic scan.\n");
        goto done;
    }
    const u32 capacity = total < 32 ? 32 : total + 32;
    NewsRecord *records = calloc(capacity, sizeof(*records));
    if (!records)
        goto done;
    u32 returned = 0;
    Result rc = newsDatabaseGetList(&db, records, capacity, "1", "received_at DESC", &returned, 0);
    log_line("Scan total %lu, returned %lu, capacity %lu.\n",
        (unsigned long)total, (unsigned long)returned, (unsigned long)capacity);
    if (!result("inspect: list records", rc) || returned != total || returned > capacity) {
        free(records);
        goto done;
    }
    unsigned found = 0, saved = 0;
    u64 scanned = 0;
    for (u32 i = 0; i < returned; i++) {
        if (!appletMainLoop())
            break;
        if (!records[i].news_id[0] || !memchr(records[i].news_id, 0, sizeof(records[i].news_id)) ||
            !memchr(records[i].user_id, 0, sizeof(records[i].user_id))) {
            log_line("Record %lu has unusable ID fields; skipped.\n", (unsigned long)i);
            continue;
        }
        log_line("Record %lu: News ID %.*s; user field: %s\n", (unsigned long)i,
            (int)sizeof(records[i].news_id), records[i].news_id,
            records[i].user_id[0] ? "populated" : "empty");
        NewsDataService data = {0};
        rc = newsCreateNewsDataService(&data);
        if (R_FAILED(rc)) {
            result("inspect: create data reader", rc);
            break;
        }
        rc = newsDataOpenWithNewsRecord(&data, &records[i]);
        if (R_FAILED(rc)) {
            log_line("Record %lu: ", (unsigned long)i);
            result("inspect: open", rc);
            newsDataClose(&data);
            continue;
        }
        u64 size = 0;
        rc = newsDataGetSize(&data, &size);
        if (R_FAILED(rc) || !size || size > 4 * 1024 * 1024 || scanned + size > 32 * 1024 * 1024) {
            log_line("Record %lu unavailable or outside scan size limit.\n", (unsigned long)i);
            newsDataClose(&data);
            continue;
        }
        unsigned char *buffer = malloc((size_t)size);
        if (!buffer) {
            newsDataClose(&data);
            break;
        }
        u64 offset = 0;
        bool ok = true;
        while (offset < size) {
            size_t want = size - offset < 65536 ? (size_t)(size - offset) : 65536;
            u64 got = 0;
            rc = newsDataRead(&data, &got, offset, buffer + offset, want);
            if (R_FAILED(rc) || got == 0 || got > want) {
                log_line("Record %lu read incomplete (result %08lX).\n", (unsigned long)i, (unsigned long)rc);
                ok = false;
                break;
            }
            offset += got;
        }
        scanned += size;
        newsDataClose(&data);
        const bool reference = strcmp(records[i].news_id, "LS00000000000000010000") == 0 && !records[i].user_id[0];
        if (ok && (reference || contains_text(buffer, (size_t)size, "Hello from OpenPak") ||
                   contains_text(buffer, (size_t)size, "OpenPak image and body test") ||
                   contains_text(buffer, (size_t)size, "OpenPak compact-format test") ||
                   contains_text(buffer, (size_t)size, "OpenPak local-format test") ||
                   contains_text(buffer, (size_t)size, "OpenPak test retired"))) {
            found++;
            log_line("Export candidate %u at record %lu; ID length %u.\n",
                found, (unsigned long)i, (unsigned)strlen(records[i].news_id));
            log_line("Stored News ID: %.*s; user-ID field populated: %s\n",
                (int)sizeof(records[i].news_id), records[i].news_id,
                records[i].user_id[0] ? "yes" : "no");
            log_line("Compared with embedded local-format payload: %s\n",
                size == sizeof(test_article) && memcmp(buffer, test_article, sizeof(test_article)) == 0 ? "MATCH" : "DIFFERENT");
            if (save_candidate(buffer, (size_t)size, found, reference))
                saved++;
        }
        free(buffer);
    }
    log_line("Scan complete: %u candidates; %u exports.\n", found, saved);
    snapshot_needed = true;
    free(records);
 done:
    newsDatabaseClose(&db);
    newsExit();
    if (snapshot_needed)
        export_database();
}

// The test's numeric ID becomes an LS-prefixed local record. Never authorize
// a mutation from a title match or write to the exported database.
#define MANAGED_ID "LS00000000001909260910"

static bool approved_payload(const unsigned char *data, size_t size) {
    return (size == sizeof(test_article) && memcmp(data, test_article, size) == 0) ||
        (size == sizeof(cleanup_1909260910) && memcmp(data, cleanup_1909260910, size) == 0);
}

// Reads and verifies the entire bounded inventory before permitting a write.
// A record at this ID with unexpected bytes or a user ID aborts the operation.
static bool inspect_managed(bool *present, bool *live) {
    *present = false;
    *live = false;
    if (!result("managed: news:v initialize", newsInitialize(NewsServiceType_Viewer)))
        return false;
    NewsDatabaseService db = {0};
    bool ok = false;
    NewsRecord *records = NULL;
    if (!result("managed: database", newsCreateNewsDatabaseService(&db)))
        goto done;
    u32 total = 0, returned = 0;
    if (!result("managed: count", newsDatabaseCount(&db, "1", &total)) || total > 512)
        goto done;
    u32 capacity = total < 32 ? 32 : total + 32;
    records = calloc(capacity, sizeof(*records));
    if (!records)
        goto done;
    if (!result("managed: list", newsDatabaseGetList(&db, records, capacity,
            "1", "received_at DESC", &returned, 0)) || returned != total || returned > capacity)
        goto done;
    log_line("Managed scan: %lu records.\n", (unsigned long)returned);
    for (u32 i = 0; i < returned; i++) {
        if (!records[i].news_id[0] || !memchr(records[i].news_id, 0, sizeof(records[i].news_id)) ||
            !memchr(records[i].user_id, 0, sizeof(records[i].user_id))) {
            log_line("Invalid record identity; no write permitted.\n");
            goto done;
        }
        if (strcmp(records[i].news_id, MANAGED_ID) != 0)
            continue;
        if (*present || records[i].user_id[0]) {
            log_line("Ambiguous target or populated user ID; no write permitted.\n");
            goto done;
        }
        *present = true;
        NewsDataService data = {0};
        if (!result("managed: data", newsCreateNewsDataService(&data)))
            goto done;
        u64 size = 0;
        bool read_ok = result("managed: open", newsDataOpenWithNewsRecord(&data, &records[i])) &&
            result("managed: size", newsDataGetSize(&data, &size));
        unsigned char *buffer = NULL;
        if (read_ok && (size == sizeof(test_article) || size == sizeof(cleanup_1909260910))) {
            buffer = malloc((size_t)size);
            if (buffer) {
                u64 offset = 0;
                while (offset < size) {
                    u64 got = 0;
                    Result rc = newsDataRead(&data, &got, offset, buffer + offset, (size_t)(size - offset));
                    if (R_FAILED(rc) || !got || got > size - offset) {
                        result("managed: read", rc);
                        read_ok = false;
                        break;
                    }
                    offset += got;
                }
            } else read_ok = false;
        } else read_ok = false;
        newsDataClose(&data);
        bool approved = read_ok && approved_payload(buffer, (size_t)size);
        if (approved)
            *live = size == sizeof(test_article) && memcmp(buffer, test_article, size) == 0;
        free(buffer);
        if (!approved) {
            log_line("Target has unknown content; refusing to change it.\n");
            goto done;
        }
        log_line("Target verified byte-for-byte: %s.\n", *live ? "current live fixture" : "current expired fixture");
    }
    log_line("Target " MANAGED_ID ": %s\n", *present ? "PRESENT" : "ABSENT");
    ok = true;
 done:
    free(records);
    newsDatabaseClose(&db);
    newsExit();
    return ok;
}

static void manage_article(bool cleanup) {
    bool present = false, live = false;
    if (!inspect_managed(&present, &live))
        return;
    if (cleanup && !present) {
        log_line("Test already absent; nothing posted.\n");
        return;
    }
    if (!cleanup && live) {
        log_line("Exact test already stored; nothing posted.\n");
        return;
    }
    const long long now = (long long)time(NULL);
    if (now < TEST_PUBLISHED - 300 || (!cleanup && now >= TEST_EXPIRES)) {
        log_line("Console time outside this fixture's window; rebuild before posting.\n");
        return;
    }
    log_line("%s: reusing test ID " MANAGED_ID " only.\n", cleanup ? "Expiry cleanup" : "Local schema test");
    if (!result("write: news:p initialize", newsInitialize(NewsServiceType_Post)))
        return;
    Result rc = cleanup ? newsPostLocalNews(cleanup_1909260910, sizeof(cleanup_1909260910)) :
        newsPostLocalNews(test_article, sizeof(test_article));
    newsExit();
    if (!result("write: PostLocalNews", rc))
        return;
    if (!inspect_managed(&present, &live)) {
        log_line("Post returned success; resulting state could not be verified.\n");
        return;
    }
    if (cleanup)
        log_line(present ? "Cleanup UNCONFIRMED: target remains.\n" : "Cleanup confirmed: target absent from database.\n");
    else
        log_line(live ? "Exact local-format test stored. Open News to check text and images.\n" :
            "Post accepted but live test NOT found. Rendering is not confirmed.\n");
}

// Documented news:v command 30500: URL, transfer-memory size and copy handle.
// This exercises the real downloader; it does not set a passphrase or bypass
// verification. Subcommand names/result conventions are unofficial research.
static void download_catalog(void) {
    static bool attempted;
    if (attempted) {
        log_line("One catalog download per launch; relaunch to retry.\n");
        return;
    }
    attempted = true;
    const char *url = "https://bcat-topics-lp1.cdn.nintendo.net/api/nx/v1/topics/catalog?l=en-GB&c[]=GB&c[]=GB";
    const u64 capacity = 4 * 1024 * 1024;
    unsigned char *buffer = memalign(0x1000, capacity);
    if (!buffer) return;
    memset(buffer, 0, capacity);
    if (!result("catalog: news:v", newsInitialize(NewsServiceType_Viewer))) {
        free(buffer);
        return;
    }
    TransferMemory memory = {0};
    if (!result("catalog: transfer memory", tmemCreateFromMemory(&memory, buffer, capacity, Perm_None))) {
        free(buffer);
        newsExit();
        return;
    }
    Service download = {0};
    Event event = {0};
    u64 size = 0;
    bool completed = false, decoded = false;
    Result rc = serviceDispatchIn(newsGetServiceSession(), 30500, capacity,
        .buffer_attrs = { SfBufferAttr_HipcPointer | SfBufferAttr_In },
        .buffers = { { url, strlen(url) + 1 } },
        .in_num_handles = 1, .in_handles = { memory.handle },
        .out_num_objects = 1, .out_objects = &download);
    if (!result("catalog: create download", rc)) goto done;
    Handle handle = INVALID_HANDLE;
    rc = serviceDispatch(&download, 0, .out_handle_attrs = { SfOutHandleAttr_HipcCopy }, .out_handles = &handle);
    if (!result("catalog: completion event", rc)) goto done;
    eventLoadRemote(&event, handle, false);
    log_line("Downloading catalog; waiting at most 30 seconds.\n");
    for (unsigned i = 0; i < 300 && appletMainLoop(); i++) {
        rc = eventWait(&event, 100000000);
        if (R_SUCCEEDED(rc)) { completed = true; break; }
        if (rc != KERNELRESULT(TimedOut)) { result("catalog: wait", rc); break; }
        consoleUpdate(NULL);
    }
    if (!completed) {
        log_line("Download did not complete within diagnostic wait.\n");
        goto done;
    }
    rc = serviceDispatch(&download, 2);
    bool get_result_ok = result("catalog: GetResult IPC", rc);
    rc = serviceDispatchOut(&download, 3, size);
    bool get_size_ok = result("catalog: decoded size IPC", rc);
    log_line("Decoded size: %llu / %llu bytes.\n", (unsigned long long)size, (unsigned long long)capacity);
    decoded = get_result_ok && get_size_ok && size > 0 && size <= capacity;
 done:
    if (serviceIsActive(&download)) {
        if (!completed) result("catalog: cancel", serviceDispatch(&download, 1));
        serviceClose(&download);
    }
    if (eventActive(&event)) eventClose(&event);
    newsExit();
    result("catalog: close memory", tmemClose(&memory));
    // The service may retain the copy briefly after cancellation. Do not read
    // or free the backing allocation until the kernel restores permissions.
    bool restored = false;
    for (unsigned i = 0; i < 50; i++) {
        MemoryInfo info = {0}; u32 page = 0;
        rc = svcQueryMemory(&info, &page, (u64)buffer);
        if (R_FAILED(rc)) break;
        if ((info.perm & Perm_Rw) == Perm_Rw) { restored = true; break; }
        svcSleepThread(100000000);
    }
    if (!restored) {
        log_line("Buffer still owned by service; retained untouched until process exit.\n");
        return;
    }
    if (decoded) {
        const char *path = "sdmc:/switch/openpak-news-probe/catalog-v013.msgpack";
        int fd = open(path, O_WRONLY | O_CREAT | O_EXCL, 0600);
        if (fd < 0) log_line("Catalog export not created (errno %d); existing file preserved.\n", errno);
        else {
            FILE *out = fdopen(fd, "wb");
            if (!out) { close(fd); unlink(path); }
            else {
                bool ok = fwrite(buffer, 1, (size_t)size, out) == size;
                if (fclose(out) != 0) ok = false;
                if (!ok) unlink(path);
                log_line(ok ? "Saved catalog-v013.msgpack; format still needs host validation.\n" : "Catalog export failed.\n");
            }
        }
    } else log_line("No decoded catalog exported. No articles or subscriptions changed by probe.\n");
    free(buffer);
}

// Read-only inspection of the documented News system save. File contents
// are exported only for names suggesting catalog/topic/detail metadata.
static unsigned cache_entries, cache_exports;
static u64 cache_bytes;

static bool cache_candidate(const char *path) {
    char lower[FS_MAX_PATH];
    size_t n = strlen(path);
    if (n >= sizeof(lower)) return false;
    for (size_t i = 0; i <= n; i++) {
        char c = path[i];
        lower[i] = c >= 'A' && c <= 'Z' ? c + ('a' - 'A') : c;
    }
    if (strstr(lower, "passphrase") || strstr(lower, "token") || strstr(lower, "secret") || strstr(lower, "key"))
        return false;
    return strstr(lower, "catalog") || strstr(lower, "topic") || strstr(lower, "detail");
}

static void cache_export(FsFileSystem *fs, const char *path) {
    if (cache_exports >= 64) return;
    FsFile file = {0};
    if (!result("cache: open candidate", fsFsOpenFile(fs, path, FsOpenMode_Read, &file))) return;
    s64 size = 0;
    if (!result("cache: candidate size", fsFileGetSize(&file, &size)) || size <= 0 || size > 4*1024*1024 ||
        cache_bytes + (u64)size > 16*1024*1024) {
        fsFileClose(&file); return;
    }
    unsigned char *buffer = malloc((size_t)size);
    if (!buffer) { fsFileClose(&file); return; }
    u64 offset = 0;
    bool ok = true;
    while (offset < (u64)size) {
        u64 got = 0, want = (u64)size - offset;
        if (want > 65536) want = 65536;
        Result rc = fsFileRead(&file, offset, buffer + offset, want, FsReadOption_None, &got);
        if (R_FAILED(rc) || !got || got > want) { result("cache: read candidate", rc); ok = false; break; }
        offset += got;
    }
    fsFileClose(&file);
    cache_bytes += (u64)size;
    if (ok) {
        char dest[128];
        snprintf(dest, sizeof(dest), "sdmc:/switch/openpak-news-probe/cache-v014-%03u.bin", ++cache_exports);
        int fd = open(dest, O_WRONLY | O_CREAT | O_EXCL, 0600);
        if (fd < 0) log_line("Export preserved/not created: %s (errno %d).\n", dest, errno);
        else {
            FILE *out = fdopen(fd,"wb");
            if (!out) { close(fd); unlink(dest); }
            else {
                ok = fwrite(buffer,1,(size_t)size,out) == (size_t)size;
                if (fclose(out) != 0) ok = false;
                if (!ok) unlink(dest);
                log_line("Cache export %s: %s <- %s\n", ok ? "saved" : "failed", dest, path);
            }
        }
    }
    free(buffer);
}

static void cache_walk(FsFileSystem *fs, const char *path, unsigned depth) {
    if (depth > 4 || cache_entries >= 512 || !appletMainLoop()) return;
    FsDir dir = {0};
    if (!result("cache: open directory", fsFsOpenDirectory(fs, path,
        FsDirOpenMode_ReadDirs | FsDirOpenMode_ReadFiles, &dir))) return;
    while (cache_entries < 512 && appletMainLoop()) {
        FsDirectoryEntry entry = {0};
        s64 count = 0;
        Result rc = fsDirRead(&dir, &count, 1, &entry);
        if (R_FAILED(rc)) { result("cache: read directory", rc); break; }
        if (!count) break;
        if (count != 1 || !memchr(entry.name, 0, sizeof(entry.name)) || !entry.name[0] ||
            strchr(entry.name, '/') || strcmp(entry.name, ".") == 0 || strcmp(entry.name, "..") == 0) {
            log_line("Unusable cache entry; stopping directory.\n"); break;
        }
        cache_entries++;
        char child[FS_MAX_PATH];
        int n = snprintf(child, sizeof(child), "%s%s%s", path, strcmp(path,"/") ? "/" : "", entry.name);
        if (n < 0 || (size_t)n >= sizeof(child)) continue;
        log_line("Cache %s %lld bytes: %s\n", entry.type == FsDirEntryType_Dir ? "directory" : "file",
            (long long)entry.file_size, child);
        if (entry.type == FsDirEntryType_Dir) cache_walk(fs, child, depth + 1);
        else if (entry.type == FsDirEntryType_File && cache_candidate(child)) cache_export(fs, child);
    }
    fsDirClose(&dir);
}

static void inspect_cache(void) {
    log_line("Inspecting News save 0x8000000000000090 using read-only operations.\n");
    FsFileSystem fs = {0};
    AccountUid uid = {0};
    Result rc = fsOpen_SystemSaveData(&fs, FsSaveDataSpaceId_System, 0x8000000000000090ULL, uid);
    if (!result("cache: open System space", rc)) {
        rc = fsOpen_SystemSaveData(&fs, FsSaveDataSpaceId_User, 0x8000000000000090ULL, uid);
        if (!result("cache: open User space", rc)) {
            log_line("News save unavailable; no files changed.\n"); return;
        }
    }
    cache_entries = cache_exports = 0; cache_bytes = 0;
    cache_walk(&fs,"/",0);
    fsFsClose(&fs);
    log_line("Cache inspection ended: %u entries, %u candidate export attempts.\n",cache_entries,cache_exports);
    log_line("Limits: depth 4, 512 entries, 64 exports, 4 MiB/file, 16 MiB total.\n");
}

#include "sync.h"
#include "native_receive.h"

int main(void) {
    consoleInit(NULL);
    padConfigureInput(1, HidNpadStyleSet_NpadStandard);
    PadState pad;
    padInitializeDefault(&pad);

    const char *dir = "sdmc:/switch/openpak-news-probe";
    bool directory_ok = mkdir(dir, 0777) == 0 || errno == EEXIST;
    if (directory_ok)
        report = fopen("sdmc:/switch/openpak-news-probe/report.txt", "a");
    if (!report)
        log_line("Cannot save report; results remain on screen.\n");
    log_line("\n--- Probe run ---\n");
    probe();
    log_line("\nFinished. No News data changed.\n");
    if (report)
        log_line("Report: /switch/openpak-news-probe/report.txt\n");
    log_line("X: inspect News cache and export channel-metadata candidates.\nZL: previous native catalog download diagnostic.\n");
    log_line("Exports contain existing News data; keep them private.\n");
    log_line("B: post local-format test and export results.\nA: expire only the verified current OpenPak test.\nY: inspect/export records (read only).\n");
    log_line("Channel: " TEST_NEWS_TOPIC " (Nintendo News).\n");

    log_line("ZR: diagnose DNS, TCP and TLS to OpenPak.\n");
    log_line("Press + to exit.\n");
    log_line("R: install OpenPak BCAT key patch. L: remove patch. Reboot required.\n");
    log_line("D-pad Right: install News token omission. D-pad Left: remove it. Reboot required.\n");
    log_line("Patch affects News and game BCAT; server containers still needed.\n");
    log_line("D-pad Down: subscribe and let the firmware receive OpenPak news.\n");
    log_line("  (ZR + D-pad Down unsubscribes only that topic.)\n");
    log_line("D-pad Up: fallback sync — homebrew downloads and posts locally.\n");
    while (appletMainLoop()) {
        padUpdate(&pad);
        if (padGetButtonsDown(&pad) & HidNpadButton_Plus)
            break;
        if (padGetButtonsDown(&pad) & HidNpadButton_ZR)
            transport_probe();
        if (padGetButtonsDown(&pad) & HidNpadButton_R)
            manage_patch(false, false);
        if (padGetButtonsDown(&pad) & HidNpadButton_L)
            manage_patch(true, false);
        if (padGetButtonsDown(&pad) & HidNpadButton_Right)
            manage_patch(false, true);
        if (padGetButtonsDown(&pad) & HidNpadButton_Left)
            manage_patch(true, true);
        if (padGetButtonsDown(&pad) & HidNpadButton_Up)
            sync_articles();
        if (padGetButtonsDown(&pad) & HidNpadButton_Down) {
            if (padGetButtons(&pad) & HidNpadButton_ZR)
                native_unsubscribe();
            else
                native_receive();
        }
        if (padGetButtonsDown(&pad) & HidNpadButton_X)
            inspect_cache();
        if (padGetButtonsDown(&pad) & HidNpadButton_ZL)
            download_catalog();
        if (padGetButtonsDown(&pad) & HidNpadButton_A)
            manage_article(true);
        if (padGetButtonsDown(&pad) & HidNpadButton_B) {
            manage_article(false);
            inspect_stored_articles();
        }
        if (padGetButtonsDown(&pad) & HidNpadButton_Y)
            inspect_stored_articles();
        consoleUpdate(NULL);
    }
    if (report) {
        fclose(report);
        report = NULL;
    }
    consoleExit(NULL);
    return 0;
}
