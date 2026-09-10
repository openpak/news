// SPDX-License-Identifier: AGPL-3.0-only
package newsformat

import (
	"bytes"
	"testing"
)

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := Encode(v)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return b
}

func TestPackPrimitives(t *testing.T) {
	cases := []struct {
		in   any
		want []byte
	}{
		{false, []byte{0xc2}},
		{true, []byte{0xc3}},
		{int64(0), []byte{0x00}},
		{int64(127), []byte{0x7f}},
		{int64(128), []byte{0xcc, 0x80}},
		{int64(255), []byte{0xcc, 0xff}},
		{int64(256), []byte{0xcd, 0x01, 0x00}},
		{int64(65535), []byte{0xcd, 0xff, 0xff}},
		{int64(65536), []byte{0xce, 0x00, 0x01, 0x00, 0x00}},
		// Timestamp-sized values must stay uint32, matching the compact
		// widths the console's own payloads use.
		{int64(1789000000), []byte{0xce, 0x6a, 0xa1, 0xf9, 0x40}},
		{int64(-1), []byte{0xff}},
		{int64(-33), []byte{0xd0, 0xdf}},
		{"", []byte{0xa0}},
		{"abc", []byte{0xa3, 'a', 'b', 'c'}},
		{[]byte{}, []byte{0xc4, 0x00}},
		{[]byte{0x01, 0x02}, []byte{0xc4, 0x02, 0x01, 0x02}},
	}
	for _, tc := range cases {
		if got := mustEncode(t, tc.in); !bytes.Equal(got, tc.want) {
			t.Errorf("Encode(%v) = % x, want % x", tc.in, got, tc.want)
		}
	}
}

func TestPackContainers(t *testing.T) {
	if got := mustEncode(t, []any{}); !bytes.Equal(got, []byte{0x90}) {
		t.Errorf("empty array = % x", got)
	}
	if got := mustEncode(t, Map{}); !bytes.Equal(got, []byte{0x80}) {
		t.Errorf("empty map = % x", got)
	}
	got := mustEncode(t, Map{{"a", int64(1)}, {"b", "xy"}})
	want := []byte{0x82, 0xa1, 'a', 0x01, 0xa1, 'b', 0xa2, 'x', 'y'}
	if !bytes.Equal(got, want) {
		t.Errorf("map = % x, want % x", got, want)
	}
	// 17 entries must use map16, preserving insertion order.
	big := Map{}
	for i := 0; i < 17; i++ {
		big = append(big, Field{Key: string(rune('a' + i)), Value: int64(i)})
	}
	got = mustEncode(t, big)
	if got[0] != 0xde {
		t.Fatalf("17-entry map header = %x, want de (map16)", got[0])
	}
}

func TestPackRejectsUnknown(t *testing.T) {
	if _, err := Encode(1.5); err == nil {
		t.Fatal("float accepted")
	}
	if _, err := Encode(map[string]any{"a": 1}); err == nil {
		t.Fatal("plain Go map accepted")
	}
}

func TestCatalogRecordOrder(t *testing.T) {
	ch := Channel{TopicID: "t", Name: "n", Publisher: "p", Description: "d",
		Important: false, PublishingTime: 10, LastPostedAt: 20}
	want := []string{"topic_id", "name", "publisher", "description", "important",
		"publishing_time", "last_posted_at"}
	assertOrder(t, ch.record(), want)

	detail := Detail(ch)
	wantDetail := append(append([]string{}, want...), "latest_news_urls")
	assertOrder(t, detail, wantDetail)
	if len(detail[len(detail)-1].Value.([]any)) != 0 {
		t.Fatal("latest_news_urls not empty")
	}

	arch := Archives()
	assertOrder(t, arch, []string{"na_required", "data_list"})
	if arch[0].Value != false || len(arch[1].Value.([]any)) != 0 {
		t.Fatal("archives payload not the validated empty shape")
	}
}

func TestArticlePayloadOrder(t *testing.T) {
	a := Article{
		NewsID: 123, PublishedAt: 1000, ExpireAt: 2000,
		Language: "en-GB", SupportedLanguages: []string{"en-GB", "en-US"},
		TopicID: "t", TopicName: "T", TopicPublisher: "p", TopicDescription: "d",
		TopicImage: []byte{1}, Title: "hi", ListImage: []byte{2}, Footer: "f",
		BodyText: "b", BodyImage: []byte{3}, BodyImageHeight: 364,
	}
	p := a.Payload()
	want := []string{"version", "news_id", "published_at", "expire_at", "pickup_limit",
		"priority", "deletion_priority", "language", "supported_languages",
		"display_type", "topic_id", "no_photography", "surprise",
		"essential_pickup_limit", "movie", "subject", "topic_name", "topic_image",
		"topic_publisher", "topic_description", "list_image", "footer", "body"}
	assertOrder(t, p, want)
	if got := p[1].Value.(int64); got != 123 {
		t.Fatalf("news_id = %d", got)
	}
	subject := p[15].Value.(Map)
	assertOrder(t, subject, []string{"caption", "text"})
	body := p[22].Value.(Map)
	assertOrder(t, body, []string{"text", "main_image_height", "main_image"})

	// Expired variant keeps identity and shrinks the window.
	e := a.ExpiredPayload()
	if got := e[1].Value.(int64); got != 123 {
		t.Fatalf("expired news_id = %d", got)
	}
	if got := e[2].Value.(int64); got != 1000-86400 {
		t.Fatalf("expired published_at = %d", got)
	}
	if got := e[3].Value.(int64); got != 999 {
		t.Fatalf("expired expire_at = %d", got)
	}
	if got := e[4].Value.(int64); got != 999 {
		t.Fatalf("expired pickup_limit = %d", got)
	}
}

func TestEncodeDeterminism(t *testing.T) {
	ch := Channel{TopicID: "t", Name: "n", Publisher: "p", Description: "d",
		PublishingTime: 1, LastPostedAt: 2}
	a := Article{NewsID: 5, PublishedAt: 3, ExpireAt: 4, Language: "en-GB",
		SupportedLanguages: []string{"en-GB"}, TopicID: "t", TopicName: "T",
		TopicPublisher: "p", TopicDescription: "d", TopicImage: []byte{9},
		Title: "x", ListImage: []byte{8}, Footer: "f", BodyText: "b",
		BodyImage: []byte{7}, BodyImageHeight: 364}
	first := mustEncode(t, Catalog([]Channel{ch}))
	for i := 0; i < 3; i++ {
		again := mustEncode(t, Catalog([]Channel{ch}))
		if !bytes.Equal(first, again) {
			t.Fatal("catalog encoding not deterministic")
		}
	}
	if !bytes.Equal(mustEncode(t, a.Payload()), mustEncode(t, a.Payload())) {
		t.Fatal("article encoding not deterministic")
	}
}

func assertOrder(t *testing.T, m Map, want []string) {
	t.Helper()
	if len(m) != len(want) {
		t.Fatalf("map has %d fields, want %d", len(m), len(want))
	}
	for i, key := range want {
		if m[i].Key != key {
			t.Fatalf("field %d = %q, want %q", i, m[i].Key, key)
		}
	}
}
