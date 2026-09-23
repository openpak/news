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
// alone cannot subscribe.
//
// The topic argument is the bare topic id (hardware, probe v0.21: both
// topic_id='...' spellings fail with 0x47d). Subscription status values
// (22.5.0 firmware): 0 = forget the topic, 1 = unsubscribed, 2 =
// subscribed, 3 = auto-subscribed. The bcat worker only fetches topics in
// status 2 and refuses to store records for any other (0xd47d) — v0.21
// wrote 1, which is why nothing it received was ever stored.
//
// Include after main.c's log_line/result.

#define RECEIVE_TOPIC "openpak_news"

#define SUBSCRIPTION_UNSUBSCRIBED 1
#define SUBSCRIPTION_SUBSCRIBED   2

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

    const char *filter = RECEIVE_TOPIC;
    u32 status = 0;
    Result rc = newsGetSubscriptionStatus(filter, &status);
    log_line("GetSubscriptionStatus %s: 0x%x (%d-%d), status=%lu\n", filter,
        rc, R_MODULE(rc), R_DESCRIPTION(rc), (unsigned long)status);
    if (!result("receive: SetSubscriptionStatus(2)", newsSetSubscriptionStatus(filter, SUBSCRIPTION_SUBSCRIBED))) {
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
    log_line("Correlate this run with the server log: /api/nx/v1/list/" RECEIVE_TOPIC ", then the article URLs it names.\n");
    log_line("To undo: press D-pad Down with ZR held to unsubscribe this topic.\n");
}

// Unsubscribe only our own topic (status 1 = unsubscribed); never
// ClearSubscriptionStatusAll, which would drop the console's Nintendo
// channels too.
static void native_unsubscribe(void) {
    if (!result("unsubscribe: news:a initialize", newsInitialize(NewsServiceType_Administrator)))
        return;
    if (result("unsubscribe: SetSubscriptionStatus(1)", newsSetSubscriptionStatus(RECEIVE_TOPIC, SUBSCRIPTION_UNSUBSCRIBED)))
        log_line("Unsubscribed: %s\n", RECEIVE_TOPIC);
    newsExit();
}
