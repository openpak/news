# SPDX-License-Identifier: AGPL-3.0-only
ifeq ($(strip $(DEVKITPRO)),)
$(error DEVKITPRO must point to the devkitPro toolchain)
endif

include $(DEVKITPRO)/libnx/switch_rules

TARGET := openpak-news-probe
ARCH := -march=armv8-a+crc+crypto -mtune=cortex-a57 -mtp=soft -fPIE
CFLAGS := $(ARCH) -O2 -g -Wall -Wextra -Werror -ffunction-sections -D__SWITCH__ -I$(LIBNX)/include
LDFLAGS := $(ARCH) -specs=$(LIBNX)/switch.specs -L$(LIBNX)/lib

.PHONY: all clean
all: $(TARGET).nro

$(TARGET).elf: source/main.c source/test_article.h source/cleanup_articles.h source/news_patch.h source/transport.h source/openpak_ca.h Makefile
	$(CC) $(CFLAGS) $< $(LDFLAGS) -lnx -o $@

$(TARGET).nacp: Makefile
	nacptool --create "OpenPak News Probe" "OpenPak" "0.19.0" $@

$(TARGET).nro: $(TARGET).elf $(TARGET).nacp
	elf2nro $< $@ --nacp=$(TARGET).nacp

clean:
	$(RM) $(TARGET).elf $(TARGET).nacp $(TARGET).nro
