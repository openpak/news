// SPDX-License-Identifier: AGPL-3.0-only
// Package newsformat encodes platform-neutral News content into the
// MessagePack payload shapes the native Switch News reader accepts.
//
// Every shape reproduces, field for field, a layout hardware has accepted:
// the NORMAL article confirmed rendering in the v0.12 local test
// (docs/native-news-local-format.md) and the catalog/detail/archives
// records the console's own service decoded on 2026-09-09
// (docs/native-news-container.md). Map ordering is preserved exactly and
// integers use compact widths, matching the observed encoder. The package
// holds no keys and no container logic; wrapping into signed/encrypted
// BCAT containers is internal/bcatcontainer's job.
package newsformat

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Field is one ordered map entry. The console reads maps by key, but every
// payload hardware has accepted used these exact orderings, so builders
// emit them deterministically.
type Field struct {
	Key   string
	Value any
}

// Map is an ordered MessagePack map.
type Map []Field

// ---------- encoder ----------

// Encode serialises v using only the types the News payloads need.
func Encode(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := pack(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func writeBytes(w io.Writer, b []byte) error {
	n, err := w.Write(b)
	if err == nil && n != len(b) {
		return io.ErrShortWrite
	}
	return err
}

func lengthHeader(w io.Writer, n int, fixBase byte, fixLimit int, tag8, tag16, tag32 byte) error {
	if n < fixLimit {
		return writeBytes(w, []byte{fixBase | byte(n)})
	}
	if n <= 255 && tag8 != 0 {
		return writeBytes(w, []byte{tag8, byte(n)})
	}
	if n <= 65535 {
		b := []byte{tag16, 0, 0}
		binary.BigEndian.PutUint16(b[1:], uint16(n))
		return writeBytes(w, b)
	}
	if uint64(n) > uint64(^uint32(0)) {
		return fmt.Errorf("newsformat value too large")
	}
	b := []byte{tag32, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[1:], uint32(n))
	return writeBytes(w, b)
}

func pack(w io.Writer, v any) error {
	switch x := v.(type) {
	case bool:
		if x {
			return writeBytes(w, []byte{0xc3})
		}
		return writeBytes(w, []byte{0xc2})
	case int:
		return pack(w, int64(x))
	case int64:
		if x >= -32 && x <= 127 {
			return writeBytes(w, []byte{byte(x)})
		}
		var b []byte
		switch {
		case x >= 0 && x <= 255:
			b = []byte{0xcc, byte(x)}
		case x >= 0 && x <= 65535:
			b = []byte{0xcd, 0, 0}
			binary.BigEndian.PutUint16(b[1:], uint16(x))
		case x >= 0 && x <= 4294967295:
			b = make([]byte, 5)
			b[0] = 0xce
			binary.BigEndian.PutUint32(b[1:], uint32(x))
		case x >= 0:
			b = make([]byte, 9)
			b[0] = 0xcf
			binary.BigEndian.PutUint64(b[1:], uint64(x))
		case x >= -128:
			b = []byte{0xd0, byte(x)}
		case x >= -32768:
			b = []byte{0xd1, 0, 0}
			binary.BigEndian.PutUint16(b[1:], uint16(x))
		case x >= -2147483648:
			b = make([]byte, 5)
			b[0] = 0xd2
			binary.BigEndian.PutUint32(b[1:], uint32(x))
		default:
			b = make([]byte, 9)
			b[0] = 0xd3
			binary.BigEndian.PutUint64(b[1:], uint64(x))
		}
		return writeBytes(w, b)
	case string:
		if err := lengthHeader(w, len(x), 0xa0, 32, 0xd9, 0xda, 0xdb); err != nil {
			return err
		}
		return writeBytes(w, []byte(x))
	case []byte:
		if err := lengthHeader(w, len(x), 0, 0, 0xc4, 0xc5, 0xc6); err != nil {
			return err
		}
		return writeBytes(w, x)
	case []any:
		if err := lengthHeader(w, len(x), 0x90, 16, 0, 0xdc, 0xdd); err != nil {
			return err
		}
		for _, child := range x {
			if err := pack(w, child); err != nil {
				return err
			}
		}
	case Map:
		if err := lengthHeader(w, len(x), 0x80, 16, 0, 0xde, 0xdf); err != nil {
			return err
		}
		for _, entry := range x {
			if err := pack(w, entry.Key); err != nil {
				return err
			}
			if err := pack(w, entry.Value); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported newsformat type %T", v)
	}
	return nil
}

// ---------- payload builders ----------

// Channel is one catalog channel record.
type Channel struct {
	TopicID        string
	Name           string
	Publisher      string
	Description    string
	Important      bool
	PublishingTime int64 // UTC Unix seconds
	LastPostedAt   int64 // UTC Unix seconds
}

func (ch Channel) record() Map {
	return Map{
		{"topic_id", ch.TopicID},
		{"name", ch.Name},
		{"publisher", ch.Publisher},
		{"description", ch.Description},
		{"important", ch.Important},
		{"publishing_time", ch.PublishingTime},
		{"last_posted_at", ch.LastPostedAt},
	}
}

// Catalog encodes the topic catalog: an array of channel records.
func Catalog(channels []Channel) []any {
	out := make([]any, 0, len(channels))
	for _, ch := range channels {
		out = append(out, ch.record())
	}
	return out
}

// Detail encodes one channel's detail record. latest_news_urls stays empty:
// its entry format has not been traced from the console's parser, and the
// payload must not invent URL fields.
func Detail(ch Channel) Map {
	return append(ch.record(), Field{Key: "latest_news_urls", Value: []any{}})
}

// Archives encodes the channel archives response. data_list stays empty
// until its entry schema is validated on hardware; see the authoring guide.
func Archives() Map {
	return Map{
		{"na_required", false},
		{"data_list", []any{}},
	}
}

// Article describes one NORMAL article in the locally proven layout.
type Article struct {
	NewsID      int64
	PublishedAt int64 // UTC Unix seconds
	ExpireAt    int64 // UTC Unix seconds; also written to pickup_limit

	Language           string
	SupportedLanguages []string

	TopicID          string
	TopicName        string
	TopicPublisher   string
	TopicDescription string
	TopicImage       []byte // 70x70 baseline JFIF JPEG

	Title     string
	ListImage []byte // 358x201 baseline JFIF JPEG
	Footer    string

	BodyText        string
	BodyImage       []byte // 800x364 baseline JFIF JPEG
	BodyImageHeight int64
}

// Payload encodes the live article.
func (a Article) Payload() Map { return a.payload(a.PublishedAt, a.ExpireAt) }

// ExpiredPayload encodes the retirement variant: the same news_id with a
// past validity window, used to replace a delivered article when it is
// retired. Expiry cleanup is console-scheduled, not an immediate delete.
func (a Article) ExpiredPayload() Map {
	return a.payload(a.PublishedAt-86400, a.PublishedAt-1)
}

func (a Article) payload(published, expire int64) Map {
	return Map{
		{"version", Map{{"format", int64(1)}, {"semantics", int64(1)}}},
		{"news_id", a.NewsID},
		{"published_at", published},
		{"expire_at", expire},
		{"pickup_limit", expire},
		{"priority", int64(50)},
		{"deletion_priority", int64(100)},
		{"language", a.Language},
		{"supported_languages", stringsToAny(a.SupportedLanguages)},
		{"display_type", "NORMAL"},
		{"topic_id", a.TopicID},
		{"no_photography", int64(0)},
		{"surprise", int64(0)},
		{"essential_pickup_limit", int64(0)},
		{"movie", int64(0)},
		{"subject", Map{{"caption", int64(1)}, {"text", a.Title}}},
		{"topic_name", a.TopicName},
		{"topic_image", a.TopicImage},
		{"topic_publisher", a.TopicPublisher},
		{"topic_description", a.TopicDescription},
		{"list_image", a.ListImage},
		{"footer", Map{{"text", a.Footer}}},
		{"body", Map{
			{"text", a.BodyText},
			{"main_image_height", a.BodyImageHeight},
			{"main_image", a.BodyImage},
		}},
	}
}

func stringsToAny(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}
