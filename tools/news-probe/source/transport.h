// SPDX-License-Identifier: AGPL-3.0-only
#include <arpa/inet.h>
#include <netdb.h>
#include <sys/socket.h>
#include <sys/select.h>
#include "openpak_ca.h"

extern SslServiceType __nx_ssl_service_type;

static bool ssl_would_block(Result rc) {
    return R_MODULE(rc) == 123 && R_DESCRIPTION(rc) == 204;
}

static void transport_address(struct in_addr address, const char *label) {
    const char *host = "bcat-topics-lp1.cdn.nintendo.net";
    char text[INET_ADDRSTRLEN];
    inet_ntop(AF_INET, &address, text, sizeof(text));
    log_line("Transport %s: %s:443\n", label, text);
    int fd = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
    if (fd < 0) { log_line("TCP socket errno=%d\n", errno); return; }
    SslContext ctx = {0};
    SslConnection conn = {0};
    int flags = fcntl(fd, F_GETFL, 0);
    if (flags < 0 || fcntl(fd, F_SETFL, flags | O_NONBLOCK)) goto done;
    struct sockaddr_in addr = {.sin_family=AF_INET, .sin_port=htons(443), .sin_addr=address};
    int err = connect(fd, (struct sockaddr *)&addr, sizeof(addr));
    if (err && errno != EINPROGRESS) { log_line("TCP connect errno=%d\n", errno); goto done; }
    fd_set writes; FD_ZERO(&writes); FD_SET(fd, &writes);
    struct timeval timeout = {.tv_sec=10};
    int ready = select(fd+1, NULL, &writes, NULL, &timeout);
    if (ready <= 0) { log_line("TCP wait result=%d errno=%d\n", ready, errno); goto done; }
    socklen_t size = sizeof(err);
    if (getsockopt(fd, SOL_SOCKET, SO_ERROR, &err, &size) || err) {
        log_line("TCP connection error=%d errno=%d\n", err, errno); goto done;
    }
    log_line("TCP connected.\n");
    if (!result("TLS create context", sslCreateContext(&ctx, 0x05000001U))) goto done;
    u64 cert_id = 0;
    if (!result("TLS import OpenPak CA", sslContextImportServerPki(&ctx, openpak_ca, sizeof(openpak_ca), SslCertificateFormat_Pem, &cert_id))) goto done;
    if (!result("TLS create connection", (__nx_ssl_service_type == SslServiceType_System ? sslContextCreateConnectionForSystem(&ctx, &conn) : sslContextCreateConnection(&ctx, &conn)))) goto done;
    if (!result("TLS retain socket", sslConnectionSetOption(&conn, SslOptionType_DoNotCloseSocket, true))) goto done;
    int transferred = socketSslConnectionSetSocketDescriptor(&conn, fd);
    if (transferred >= 0) close(transferred);
    else if (errno != ENOENT) { log_line("TLS attach socket errno=%d\n", errno); goto done; }
    if (!result("TLS hostname", sslConnectionSetHostName(&conn, host, strlen(host)+1))) goto done;
    if (!result("TLS nonblocking", sslConnectionSetIoMode(&conn, SslIoMode_NonBlocking))) goto done;
    u32 verify = 0;
    if (result("TLS verification options", sslConnectionGetVerifyOption(&conn, &verify)))
        log_line("TLS verify mask: %lu (CA imported for this context only).\n", (unsigned long)verify);
    Result rc = 0;
    u64 deadline = armGetSystemTick() + 15 * armGetSystemTickFreq();
    do {
        rc = sslConnectionDoHandshake(&conn, NULL, NULL, NULL, 0);
        if (!ssl_would_block(rc)) break;
        svcSleepThread(20000000);
    } while (appletMainLoop() && armGetSystemTick() < deadline);
    if (!result("TLS handshake", rc)) {
        result("TLS certificate error", sslConnectionGetVerifyCertError(&conn)); goto done;
    }
    const char request[] = "GET /api/nx/v1/topics/catalog?l=en-GB HTTP/1.1\r\nHost: bcat-topics-lp1.cdn.nintendo.net\r\nConnection: close\r\n\r\n";
    u32 sent = 0, offset = 0;
    deadline = armGetSystemTick() + 10 * armGetSystemTickFreq();
    while (offset < sizeof(request)-1 && appletMainLoop() && armGetSystemTick() < deadline) {
        rc = sslConnectionWrite(&conn, request+offset, sizeof(request)-1-offset, &sent);
        if (R_SUCCEEDED(rc)) { if (!sent) break; offset += sent; }
        else if (!ssl_would_block(rc)) break;
        svcSleepThread(20000000);
    }
    if (offset != sizeof(request)-1) { result("TLS HTTP write", rc); log_line("HTTP request incomplete.\n"); goto done; }
    unsigned char reply[512] = {0};
    u32 received = 0;
    deadline = armGetSystemTick() + 10 * armGetSystemTickFreq();
    do {
        rc = sslConnectionRead(&conn, reply, sizeof(reply), &received);
        if (!ssl_would_block(rc)) break;
        svcSleepThread(20000000);
    } while (appletMainLoop() && armGetSystemTick() < deadline);
    result("TLS HTTP read", rc);
    log_line("HTTP first read bytes=%lu, HTTP prefix=%s\n", (unsigned long)received,
        received >= 5 && !memcmp(reply, "HTTP/", 5) ? "yes" : "no");
    // No response content, credentials or certificate bytes are logged.
 done:
    if (serviceIsActive(&conn.s)) sslConnectionClose(&conn);
    if (serviceIsActive(&ctx.s)) sslContextClose(&ctx);
    close(fd);
}

static void transport_probe(void) {
    log_line("Transport diagnostic: DNS, TCP, TLS with public OpenPak CA.\n");
    if (!result("socket initialize", socketInitializeDefault())) return;

    struct addrinfo hints = {.ai_family=AF_INET, .ai_socktype=SOCK_STREAM}, *addresses = NULL;
    int err = getaddrinfo("bcat-topics-lp1.cdn.nintendo.net", "443", &hints, &addresses);
    log_line("DNS getaddrinfo result=%d\n", err);
    struct in_addr expected;
    inet_pton(AF_INET, "145.241.199.19", &expected);
    bool same = false;
    if (!err && addresses) {
        struct in_addr resolved = ((struct sockaddr_in *)addresses->ai_addr)->sin_addr;
        same = resolved.s_addr == expected.s_addr;
        log_line("DNS matches OpenPak remote: %s\n", same ? "yes" : "no");
        // Avoid sending a test to an unintended DNS destination.

    }
    if (addresses) freeaddrinfo(addresses);
    log_line("Matching native SSL profile: 0x05000001 (API 5, Auto TLS).\n");
    for (unsigned int system = 0; system < 2; ++system) {
        __nx_ssl_service_type = system ? SslServiceType_System : SslServiceType_Default;
        log_line("SSL service: %s\n", system ? "ssl:s (system)" : "ssl (default)");
        if (!result("SSL initialize", sslInitialize(1))) continue;
        transport_address(expected, same ? "DNS route" : "explicit OpenPak IP (DNS unchanged)");
        sslExit();
    }
    __nx_ssl_service_type = SslServiceType_Default;
    socketExit();
    log_line("Transport diagnostic finished. ZL still tests native News.\n");
}
