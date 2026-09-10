// SPDX-License-Identifier: AGPL-3.0-only
// newsfixture builds an original, experimental local-News article for the NRO.
// It does not read a Nintendo sample or publish anything to a console.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"

	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

const testID int64 = 1909260910

// Owner-selected existing channel, identified from their private sample.
const testTopicID = "nx_news"
const testTopicName = "Nintendo News"

func main() {
	out := flag.String("out", "test_article.h", "generated C header path")
	catalog := flag.String("catalog-dir", "", "write original decoded catalog/detail/icon fixtures (not signed CDN responses)")
	cleanup := flag.Bool("cleanup", false, "generate an expired replacement for the current test ID")
	when := flag.String("published", "", "RFC3339 publication time; default now")
	flag.Parse()
	now := time.Now().UTC().Truncate(time.Second)
	if *when != "" {
		var err error
		now, err = time.Parse(time.RFC3339, *when)
		if err != nil {
			panic(err)
		}
	}
	if *catalog != "" {
		if err := writeCatalog(*catalog, now); err != nil {
			panic(err)
		}
		return
	}
	if *cleanup {
		var header bytes.Buffer
		fmt.Fprintln(&header, "// Generated original OpenPak cleanup fixtures. SPDX-License-Identifier: AGPL-3.0-only")
		for _, id := range []int64{testID} {
			b, err := articleFor(now.Add(-24*time.Hour), id, now.Add(-time.Hour), "OpenPak test retired")
			if err != nil {
				panic(err)
			}
			fmt.Fprintf(&header, "static const unsigned char cleanup_%d[] = {\n", id)
			for i, v := range b {
				fmt.Fprintf(&header, "0x%02x,", v)
				if i%16 == 15 {
					header.WriteByte('\n')
				}
			}
			fmt.Fprintln(&header, "\n};")
		}
		if err := os.WriteFile(*out, header.Bytes(), 0644); err != nil {
			panic(err)
		}
		fmt.Println("Generated expired replacement for the current test ID only")
		return
	}
	b, err := article(now)
	if err != nil {
		panic(err)
	}
	var header bytes.Buffer
	fmt.Fprintln(&header, "// Generated original OpenPak test fixture. SPDX-License-Identifier: AGPL-3.0-only")
	fmt.Fprintf(&header, "#define TEST_NEWS_ID \"%d\"\n#define TEST_PUBLISHED %dLL\n#define TEST_EXPIRES %dLL\n", testID, now.Unix(), now.Add(7*24*time.Hour).Unix())
	fmt.Fprintf(&header, "#define TEST_NEWS_TOPIC %q\n", testTopicID)
	fmt.Fprintln(&header, "static const unsigned char test_article[] = {")
	for i, v := range b {
		fmt.Fprintf(&header, "0x%02x,", v)
		if i%16 == 15 {
			header.WriteByte('\n')
		}
	}
	fmt.Fprintln(&header, "\n};")
	if err := os.WriteFile(*out, header.Bytes(), 0644); err != nil {
		panic(err)
	}
	fmt.Printf("Original article %d: %d bytes, published %s, expires %s\n", testID, len(b), now.Format(time.RFC3339), now.Add(7*24*time.Hour).Format(time.RFC3339))
}

func graphic(w, h int) ([]byte, error) {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{17, 28, 47, 255}
			if x > w/8 && x < w*7/8 && y > h/3 && y < h*2/3 {
				c = color.RGBA{43, 212, 158, 255}
			}
			if x > w*3/8 && x < w*5/8 && y > h/8 && y < h*7/8 {
				c = color.RGBA{55, 136, 240, 255}
			}
			im.SetRGBA(x, y, c)
		}
	}
	// Match the observed JFIF baseline/4:4:4 transport, while drawing
	// all pixels ourselves. Go's JPEG encoder only emits 4:2:0 colour.
	var ppm bytes.Buffer
	fmt.Fprintf(&ppm, "P6\n%d %d\n255\n", w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := im.RGBAAt(x, y)
			ppm.Write([]byte{c.R, c.G, c.B})
		}
	}
	cmd := exec.Command("cjpeg", "-quality", "85", "-baseline", "-sample", "1x1")
	cmd.Stdin = &ppm
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cjpeg (libjpeg-turbo) is required: %w", err)
	}
	return b, nil
}

func article(now time.Time) ([]byte, error) {
	return articleFor(now, testID, now.Add(7*24*time.Hour), "OpenPak local-format test")
}

func articleFor(now time.Time, id int64, expires time.Time, title string) ([]byte, error) {
	icon, err := graphic(70, 70)
	if err != nil {
		return nil, err
	}
	card, err := graphic(358, 201)
	if err != nil {
		return nil, err
	}
	hero, err := graphic(800, 364)
	if err != nil {
		return nil, err
	}
	// Match the observed built-in local schema and field order. All prose
	// and image pixels are original; no reference article is read here.
	v := orderedMap{
		{"version", orderedMap{{"format", int64(1)}, {"semantics", int64(1)}}},
		{"news_id", id},
		{"published_at", now.Unix()}, {"expire_at", expires.Unix()},
		{"pickup_limit", expires.Unix()},
		{"priority", int64(50)}, {"deletion_priority", int64(100)},
		{"language", "en-GB"}, {"supported_languages", []any{"en-GB", "en-US"}},
		{"display_type", "NORMAL"}, {"topic_id", testTopicID},
		{"no_photography", int64(0)}, {"surprise", int64(0)},
		{"essential_pickup_limit", int64(0)}, {"movie", int64(0)},
		{"subject", orderedMap{{"caption", int64(1)}, {"text", title}}},
		{"topic_name", testTopicName}, {"topic_image", icon},
		{"topic_publisher", "OpenPak"},
		{"topic_description", "OpenPak native News hardware experiment."},
		{"list_image", card},
		{"footer", orderedMap{{"text", "Original OpenPak local News test."}}},
		{"body", orderedMap{
			{"text", "This is the OpenPak local-format test. If this text and the blue and green image appear, the native News reader can display our local article."},
			{"main_image_height", int64(364)}, {"main_image", hero},
		}},
	}
	var b bytes.Buffer
	err = pack(&b, v)
	return b.Bytes(), err
}

type field struct {
	key   string
	value any
}
type orderedMap []field

// pack supports only the types used by this fixture. It is not a public
// renderer or a general MessagePack library. Map ordering is deterministic.
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
		return fmt.Errorf("fixture value too large")
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
	case orderedMap:
		if err := lengthHeader(w, len(x), 0x80, 16, 0, 0xde, 0xdf); err != nil {
			return err
		}
		for _, entry := range x {
			if err := pack(w, entry.key); err != nil {
				return err
			}
			if err := pack(w, entry.value); err != nil {
				return err
			}
		}
	case map[string]any:
		if err := lengthHeader(w, len(x), 0x80, 16, 0, 0xde, 0xdf); err != nil {
			return err
		}
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := pack(w, key); err != nil {
				return err
			}
			if err := pack(w, x[key]); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported fixture type %T", v)
	}
	return nil
}

// Independent schema fixture based on the historical decoder's documented
// field types. These are decrypted payloads, not accepted CDN containers.
func catalogRecord(now time.Time) orderedMap {
	return orderedMap{
		{"topic_id", "openpak_news"}, {"name", "OpenPak News"},
		{"publisher", "OpenPak"}, {"description", "News from the OpenPak network."},
		{"publishing_time", now.Unix()}, {"last_posted_at", now.Unix()},
		{"important", false},
	}
}
func writeCatalog(dir string, now time.Time) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	record := catalogRecord(now)
	detail := append(append(orderedMap{}, record...), field{"latest_news_urls", []any{}})
	for _, item := range []struct {
		name  string
		value any
	}{
		{"catalog.msgpack", []any{record}}, {"detail.msgpack", detail},
		// HOS 22.5 channel-page parser: map with bool and article array.
		{"archives.msgpack", orderedMap{{"na_required", false}, {"data_list", []any{}}}},
	} {
		var b bytes.Buffer
		if err := pack(&b, item.value); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, item.name), b.Bytes(), 0644); err != nil {
			return err
		}
	}
	icon, err := graphic(70, 70)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "icon.jpg"), icon, 0644); err != nil {
		return err
	}
	fmt.Println("Decoded catalog fixtures written; encryption/signature envelope is NOT provided.")
	return nil
}
