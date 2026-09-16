// SPDX-License-Identifier: AGPL-3.0-only
// OpenPak article sync: fetch published articles over HTTPS and store them
// through newsPostLocalNews, the path hardware proved in v0.12.
//
// This path needs no BCAT container, no verification-key patch, no token
// patch and no DNS redirect: PostLocalNews takes MessagePack directly. It
// is a foreground sync — the player runs it — not background delivery.
//
// Depends on transport.h for openpak_ca and ssl_would_block, and on main.c
// for log_line, result and contains_text. Include it after those.

#define SYNC_HOST "bcat-topics-lp1.cdn.nintendo.net"
#define SYNC_ADDRESS "145.241.199.19"
#define SYNC_INDEX_MAX 8192
#define SYNC_PAYLOAD_MAX (512 * 1024)
#define SYNC_MAX_ARTICLES 32
#define SYNC_POSTED_PATH "sdmc:/switch/openpak-news-probe/posted.txt"

// One HTTPS GET to the OpenPak server, by address: the request never goes
// to whatever DNS currently resolves, and carries no console credentials.
// Returns the body only, and only for a 200 with a complete Content-Length.
static bool https_get(const char *path, unsigned char *body, size_t capacity, size_t *length) {
    *length = 0;
    bool ok = false;
    unsigned char *raw = malloc(capacity + 4096);
    if (!raw) {
        log_line("sync: out of memory for %s\n", path);
        return false;
    }
    struct in_addr address;
    inet_pton(AF_INET, SYNC_ADDRESS, &address);
    int fd = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
    if (fd < 0) { log_line("sync: socket errno=%d\n", errno); free(raw); return false; }
    SslContext ctx = {0};
    SslConnection conn = {0};
    int flags = fcntl(fd, F_GETFL, 0);
    if (flags < 0 || fcntl(fd, F_SETFL, flags | O_NONBLOCK)) goto done;
    struct sockaddr_in addr = {.sin_family=AF_INET, .sin_port=htons(443), .sin_addr=address};
    int err = connect(fd, (struct sockaddr *)&addr, sizeof(addr));
    if (err && errno != EINPROGRESS) { log_line("sync: connect errno=%d\n", errno); goto done; }
    fd_set writes; FD_ZERO(&writes); FD_SET(fd, &writes);
    struct timeval timeout = {.tv_sec=10};
    if (select(fd+1, NULL, &writes, NULL, &timeout) <= 0) { log_line("sync: connect timeout\n"); goto done; }
    socklen_t size = sizeof(err);
    if (getsockopt(fd, SOL_SOCKET, SO_ERROR, &err, &size) || err) {
        log_line("sync: connection error=%d\n", err); goto done;
    }
    if (!result("sync: TLS context", sslCreateContext(&ctx, 0x05000001U))) goto done;
    u64 cert_id = 0;
    if (!result("sync: import OpenPak CA", sslContextImportServerPki(&ctx, openpak_ca, sizeof(openpak_ca), SslCertificateFormat_Pem, &cert_id))) goto done;
    if (!result("sync: TLS connection", sslContextCreateConnection(&ctx, &conn))) goto done;
    if (!result("sync: retain socket", sslConnectionSetOption(&conn, SslOptionType_DoNotCloseSocket, true))) goto done;
    int transferred = socketSslConnectionSetSocketDescriptor(&conn, fd);
    if (transferred >= 0) close(transferred);
    else if (errno != ENOENT) { log_line("sync: attach socket errno=%d\n", errno); goto done; }
    if (!result("sync: hostname", sslConnectionSetHostName(&conn, SYNC_HOST, strlen(SYNC_HOST)+1))) goto done;
    if (!result("sync: nonblocking", sslConnectionSetIoMode(&conn, SslIoMode_NonBlocking))) goto done;
    Result rc = 0;
    u64 deadline = armGetSystemTick() + 15 * armGetSystemTickFreq();
    do {
        rc = sslConnectionDoHandshake(&conn, NULL, NULL, NULL, 0);
        if (!ssl_would_block(rc)) break;
        svcSleepThread(20000000);
    } while (appletMainLoop() && armGetSystemTick() < deadline);
    if (!result("sync: handshake", rc)) {
        result("sync: certificate error", sslConnectionGetVerifyCertError(&conn));
        goto done;
    }
    char request[512];
    int request_size = snprintf(request, sizeof(request),
        "GET %s HTTP/1.1\r\nHost: " SYNC_HOST "\r\nConnection: close\r\n\r\n", path);
    if (request_size <= 0 || (size_t)request_size >= sizeof(request)) { log_line("sync: path too long\n"); goto done; }
    u32 sent = 0, offset = 0;
    deadline = armGetSystemTick() + 10 * armGetSystemTickFreq();
    while (offset < (u32)request_size && appletMainLoop() && armGetSystemTick() < deadline) {
        rc = sslConnectionWrite(&conn, request+offset, (u32)request_size-offset, &sent);
        if (R_SUCCEEDED(rc)) { if (!sent) break; offset += sent; }
        else if (!ssl_would_block(rc)) break;
        svcSleepThread(20000000);
    }
    if (offset != (u32)request_size) { result("sync: request write", rc); goto done; }

    // Read until the declared body is complete; a short or oversized
    // response is a failure, never a partial article.
    size_t total = 0, header_end = 0, content_length = 0;
    deadline = armGetSystemTick() + 30 * armGetSystemTickFreq();
    while (appletMainLoop() && armGetSystemTick() < deadline) {
        u32 received = 0;
        rc = sslConnectionRead(&conn, raw + total, (u32)(capacity + 4096 - total), &received);
        if (ssl_would_block(rc)) { svcSleepThread(20000000); continue; }
        if (R_FAILED(rc)) { result("sync: read", rc); goto done; }
        if (!received) break;
        total += received;
        if (!header_end) {
            for (size_t i = 3; i < total; i++)
                if (!memcmp(raw + i - 3, "\r\n\r\n", 4)) { header_end = i + 1; break; }
            if (header_end) {
                raw[header_end - 1] = 0; // bounded, NUL-terminated header block
                if (memcmp(raw, "HTTP/1.1 200", 12)) {
                    log_line("sync: %s not served (status line rejected)\n", path);
                    goto done;
                }
                const char *field = strstr((char *)raw, "Content-Length:");
                if (!field) { log_line("sync: no Content-Length for %s\n", path); goto done; }
                long declared = strtol(field + 15, NULL, 10);
                if (declared <= 0 || (size_t)declared > capacity) {
                    log_line("sync: %s size %ld out of range\n", path, declared);
                    goto done;
                }
                content_length = (size_t)declared;
                raw[header_end - 1] = '\n';
            }
        }
        if (header_end && total - header_end >= content_length) break;
        if (total >= capacity + 4096) { log_line("sync: response too large\n"); goto done; }
    }
    if (!header_end || total - header_end != content_length) {
        log_line("sync: incomplete response for %s\n", path);
        goto done;
    }
    memcpy(body, raw + header_end, content_length);
    *length = content_length;
    ok = true;
 done:
    if (serviceIsActive(&conn.s)) sslConnectionClose(&conn);
    if (serviceIsActive(&ctx.s)) sslContextClose(&ctx);
    close(fd);
    free(raw);
    return ok;
}

// Already-posted IDs, so a rerun does not repost. Kept beside the report on
// the SD card; deleting it makes the next sync repost everything.
static char sync_posted[SYNC_MAX_ARTICLES * 24 + 2];

static void posted_load(void) {
    sync_posted[0] = '\n';
    sync_posted[1] = 0;
    FILE *f = fopen(SYNC_POSTED_PATH, "r");
    if (!f)
        return;
    size_t got = fread(sync_posted + 1, 1, sizeof(sync_posted) - 2, f);
    sync_posted[1 + got] = 0;
    fclose(f);
}

static bool posted_has(const char *id) {
    char needle[32];
    snprintf(needle, sizeof(needle), "\n%s\n", id);
    return strstr(sync_posted, needle) != NULL;
}

static void posted_add(const char *id) {
    FILE *f = fopen(SYNC_POSTED_PATH, "a");
    if (!f) {
        log_line("sync: cannot record %s as posted; it may repost.\n", id);
        return;
    }
    fprintf(f, "%s\n", id);
    fclose(f);
    size_t used = strlen(sync_posted);
    snprintf(sync_posted + used, sizeof(sync_posted) - used, "%s\n", id);
}

// A server response is only posted if it looks like the article layout the
// applet accepted: an ordered map with the fields the reader requires.
static bool sync_article_payload(const unsigned char *data, size_t size) {
    return size > 1024 && data[0] == 0xde &&
        contains_text(data, size, "news_id") &&
        contains_text(data, size, "display_type") &&
        contains_text(data, size, "topic_id") &&
        contains_text(data, size, "body");
}

static void sync_articles(void) {
    log_line("Sync: fetching OpenPak articles from " SYNC_ADDRESS " (local posting).\n");
    if (!result("sync: socket initialize", socketInitializeDefault()))
        return;
    if (!result("sync: SSL initialize", sslInitialize(1))) {
        socketExit();
        return;
    }
    unsigned char *index = malloc(SYNC_INDEX_MAX + 1);
    unsigned char *payload = malloc(SYNC_PAYLOAD_MAX);
    size_t index_size = 0;
    unsigned posted = 0, skipped = 0, failed = 0;
    if (!index || !payload) {
        log_line("sync: out of memory.\n");
        goto done;
    }
    if (!https_get("/openpak/v1/local/index", index, SYNC_INDEX_MAX, &index_size))
        goto done;
    index[index_size] = 0;
    posted_load();
    for (char *line = strtok((char *)index, "\n"); line; line = strtok(NULL, "\n")) {
        char topic[72], id[16];
        unsigned long declared = 0;
        if (sscanf(line, "%71s %15s %lu", topic, id, &declared) != 3) {
            log_line("sync: unreadable index line; stopping.\n");
            break;
        }
        if (posted + skipped + failed >= SYNC_MAX_ARTICLES) {
            log_line("sync: %u-article limit reached; run again for the rest.\n", SYNC_MAX_ARTICLES);
            break;
        }
        if (posted_has(id)) { skipped++; continue; }
        if (!declared || declared > SYNC_PAYLOAD_MAX) {
            log_line("sync: %s/%s declared size out of range.\n", topic, id);
            failed++;
            continue;
        }
        char path[192];
        snprintf(path, sizeof(path), "/openpak/v1/local/%s/news/%s", topic, id);
        size_t size = 0;
        if (!https_get(path, payload, SYNC_PAYLOAD_MAX, &size) || size != declared) {
            log_line("sync: %s/%s not fetched.\n", topic, id);
            failed++;
            continue;
        }
        if (!sync_article_payload(payload, size)) {
            log_line("sync: %s/%s is not an article payload; not posted.\n", topic, id);
            failed++;
            continue;
        }
        if (!result("sync: news:p initialize", newsInitialize(NewsServiceType_Post))) {
            failed++;
            break;
        }
        Result rc = newsPostLocalNews(payload, size);
        newsExit();
        if (!result("sync: PostLocalNews", rc)) {
            log_line("sync: %s/%s rejected by the News service.\n", topic, id);
            failed++;
            continue;
        }
        log_line("sync: stored %s/%s (%lu bytes).\n", topic, id, (unsigned long)size);
        posted_add(id);
        posted++;
    }
    log_line("Sync finished: %u posted, %u already present, %u failed.\n", posted, skipped, failed);
    log_line("Open News to check the articles. Delete posted.txt to repost.\n");
 done:
    free(index);
    free(payload);
    sslExit();
    socketExit();
}
