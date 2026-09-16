// SPDX-License-Identifier: AGPL-3.0-only
// Stock-firmware reception: subscribe through the firmware's own
// subscription state and ask the News module to receive now.
//
// Everything after that call is firmware: the News/BCAT module performs the
// download, container signature verification, decryption and storage, and
// the stock News applet renders the result. This NRO only flips the
// subscription and triggers a refresh — the same two things Find Channels
// does — so no article bytes pass through homebrew.
//
// Unlike the sync fallback in sync.h, this path DOES need the console's
// existing setup: the BCAT verification-key patch, the News token patch and
// OpenPak DNS. Without them the firmware's own downloader cannot fetch or
// verify OpenPak containers.
//
// Commands (libnx news.h, switchbrew BCAT services): GetSubscriptionStatus
// 30100, GetTopicList 30101, RequestImmediateReception 30300,
// SetSubscriptionStatus 40100. news:a carries every permission bit; news:v
// alone cannot subscribe. Filters are NUL-terminated strings whose accepted
// form is undocumented, so the read-only status call probes candidates
// first and only a filter naming our own topic is ever used to write.
//
// Include after main.c's log_line/result.

#define RECEIVE_TOPIC "openpak_news"

// Candidate filter spellings, read-only probe order. The database services
// take SQL-style where clauses ("1", "received_at DESC"), so the subscription
// filter may be one too; it may equally be a bare topic id.
static const char *const receive_filters[] = {
    "topic_id='" RECEIVE_TOPIC "'",
    RECEIVE_TOPIC,
    "topic_id=\"" RECEIVE_TOPIC "\"",
};

// A write must never match more than our own channel: "1" or an empty
// filter would subscribe the console to every topic the firmware knows.
static bool receive_filter_allowed(const char *filter) {
    return filter && strstr(filter, RECEIVE_TOPIC) != NULL;
}

static bool news_record_count(const char *label, u32 *out) {
    *out = 0;
    if (!result(label, newsInitialize(NewsServiceType_Viewer)))
        return false;
    NewsDatabaseService db = {0};
    bool ok = false;
    if (result("receive: database", newsCreateNewsDatabaseService(&db))) {
        ok = result("receive: count", newsDatabaseCount(&db, "1", out));
        newsDatabaseClose(&db);
    }
    newsExit();
    return ok;
}

static void native_receive(void) {
    log_line("Native reception: firmware subscription and immediate receive.\n");
    log_line("Requires the key patch, token patch and OpenPak DNS.\n");
    u32 before = 0, after = 0;
    bool counted = news_record_count("receive: news:v initialize", &before);
    if (counted)
        log_line("Records before: %lu.\n", (unsigned long)before);

    if (!result("receive: news:a initialize", newsInitialize(NewsServiceType_Administrator)))
        return;

    // Read-only: what the firmware itself believes about topics. If the
    // catalog fetch registered OpenPak, it should appear here.
    NewsTopicName topics[64];
    u32 topic_count = 0;
    if (result("receive: GetTopicList", newsGetTopicList(0, &topic_count, topics, 64))) {
        if (topic_count > 64)
            topic_count = 64;
        log_line("Firmware topic list: %lu entries.\n", (unsigned long)topic_count);
        for (u32 i = 0; i < topic_count; i++) {
            char name[sizeof(topics[i].name) + 1];
            memcpy(name, topics[i].name, sizeof(topics[i].name));
            name[sizeof(topics[i].name)] = 0;
            log_line("  topic: %s%s\n", name, strstr(name, RECEIVE_TOPIC) ? "  <- OpenPak" : "");
        }
    }

    // Read-only probe: find a filter spelling the service accepts.
    const char *filter = NULL;
    for (size_t i = 0; i < sizeof(receive_filters)/sizeof(*receive_filters); i++) {
        u32 status = 0;
        Result rc = newsGetSubscriptionStatus(receive_filters[i], &status);
        log_line("GetSubscriptionStatus %s: 0x%x (%d-%d), status=%lu\n", receive_filters[i],
            rc, R_MODULE(rc), R_DESCRIPTION(rc), (unsigned long)status);
        if (R_SUCCEEDED(rc) && !filter)
            filter = receive_filters[i];
    }
    if (!filter) {
        log_line("No filter spelling accepted; not subscribing.\n");
        log_line("Report these result codes; the accepted form is undocumented.\n");
        newsExit();
        return;
    }
    if (!receive_filter_allowed(filter)) {
        log_line("Refusing to write with a filter that is not " RECEIVE_TOPIC ".\n");
        newsExit();
        return;
    }
    log_line("Using filter: %s\n", filter);
    if (!result("receive: SetSubscriptionStatus(1)", newsSetSubscriptionStatus(filter, 1))) {
        log_line("Subscription not changed; trying reception anyway.\n");
    } else {
        u32 status = 0;
        if (result("receive: confirm subscription", newsGetSubscriptionStatus(filter, &status)))
            log_line("Subscription status now: %lu.\n", (unsigned long)status);
    }
    // The firmware downloads, verifies, decrypts and stores from here on.
    result("receive: RequestImmediateReception", newsRequestImmediateReception(filter));
    newsExit();

    // Reception is asynchronous; give the module time before counting.
    log_line("Waiting 20s for the News module to receive.\n");
    u64 deadline = armGetSystemTick() + 20 * armGetSystemTickFreq();
    while (appletMainLoop() && armGetSystemTick() < deadline) {
        consoleUpdate(NULL);
        svcSleepThread(100000000);
    }
    if (counted && news_record_count("receive: news:v recount", &after)) {
        log_line("Records after: %lu (%+ld).\n", (unsigned long)after, (long)after - (long)before);
        if (after > before)
            log_line("New records stored by the firmware. Open News to check them.\n");
        else
            log_line("No new record yet; the module may retry on its own schedule.\n");
    }
    log_line("Correlate this run with the server log: catalog, archives, summary, article.\n");
    log_line("To undo: press D-pad Down with ZR held to unsubscribe this topic.\n");
}

// Unsubscribe only our own topic; never ClearSubscriptionStatusAll, which
// would drop the console's Nintendo channels too.
static void native_unsubscribe(void) {
    if (!result("unsubscribe: news:a initialize", newsInitialize(NewsServiceType_Administrator)))
        return;
    for (size_t i = 0; i < sizeof(receive_filters)/sizeof(*receive_filters); i++) {
        u32 status = 0;
        if (R_FAILED(newsGetSubscriptionStatus(receive_filters[i], &status)))
            continue;
        if (!receive_filter_allowed(receive_filters[i]))
            continue;
        result("unsubscribe: SetSubscriptionStatus(0)", newsSetSubscriptionStatus(receive_filters[i], 0));
        log_line("Unsubscribed with filter: %s\n", receive_filters[i]);
        break;
    }
    newsExit();
}
