// SPDX-License-Identifier: AGPL-3.0-only
// Separate, pinned News memory-download edge-token omission experiment.
static const unsigned char news_auth_patch[] = {
    0x50, 0x41, 0x54, 0x43, 0x48, 0x0f, 0xc8, 0xa4, 0x00, 0x08,
    0xbf, 0x02, 0x00, 0x39, 0xe0, 0x03, 0x1f, 0x2a, 0x45, 0x4f, 0x46
};
#define NEWS_AUTH_PATCH_DIR "sdmc:/atmosphere/exefs_patches/openpak_news_no_dauth"
#define NEWS_AUTH_PATCH_PATH NEWS_AUTH_PATCH_DIR "/6D9772A733E2370B3F401EFEB7CA8F664309BD66000000000000000000000000.ips"
