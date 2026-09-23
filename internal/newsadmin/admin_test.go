// SPDX-License-Identifier: AGPL-3.0-only
package newsadmin

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openpak/news/internal/bcatcontainer"
	"github.com/openpak/news/internal/newsexperiment"
	"github.com/openpak/news/internal/newsseed"
)

const testKey = "admin-test-key"

var (
	keyOnce sync.Once
	rsaKey  *rsa.PrivateKey
)

func signingKey(t *testing.T) *rsa.PrivateKey {
	keyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		rsaKey = k
	})
	return rsaKey
}

type pushLog struct {
	mu   sync.Mutex
	got  []map[string]any
	keys []string
}

type env struct {
	t      *testing.T
	data   string
	svc    *Service
	sw     *newsexperiment.Switch
	api    http.Handler
	aesKey []byte
	pushes *pushLog
	now    time.Time
}

func writeSourceTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("lineup.json", `{"channels": [{"topic_id": "openpak_test", "name": "OpenPak Test", "publisher": "OpenPak",
		"description": "Test channel.", "important": false, "icon": {"motif": "mark"}}]}`)
	for _, a := range []struct{ id, title string }{{"2609100001", "First"}, {"2609100002", "Second"}} {
		write("openpak_test/"+a.id+".json", `{"news_id": `+a.id+`, "title": "`+a.title+`", "body": "Body.",
			"language": "en-GB", "supported_languages": ["en-GB"], "lifetime_days": 7, "art": {"motif": "burst"}}`)
	}
	write("retired.json", `{"retired": []}`)
	write("screenshots/2609100001.png", "not imported by default")
	return dir
}

func newEnv(t *testing.T, withKeys bool) *env {
	t.Helper()
	e := &env{t: t, data: t.TempDir(), sw: &newsexperiment.Switch{}, pushes: &pushLog{},
		now: time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)}
	nxbaas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		e.pushes.mu.Lock()
		e.pushes.got = append(e.pushes.got, body)
		e.pushes.keys = append(e.pushes.keys, r.Header.Get("X-Internal-Key"))
		e.pushes.mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(nxbaas.Close)
	cfg := Config{
		DataDir: e.data, ImportDir: writeSourceTree(t), KeepBuilds: 3, Switch: e.sw,
		VerifyKey: &signingKey(t).PublicKey,
		Push:      &Pusher{URL: nxbaas.URL + "/internal/news/push", Key: "nx-key"},
		Now:       func() time.Time { return e.now },
	}
	if withKeys {
		keys := t.TempDir()
		der := x509.MarshalPKCS1PrivateKey(signingKey(t))
		cfg.SigningKeyPath = filepath.Join(keys, "signing.pem")
		os.WriteFile(cfg.SigningKeyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}), 0600)
		e.aesKey = make([]byte, 32)
		rand.Read(e.aesKey)
		cfg.AESKeyPath = filepath.Join(keys, "aes.key")
		os.WriteFile(cfg.AESKeyPath, e.aesKey, 0600)
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.svc, e.api = svc, svc.Handler(testKey)
	return e
}

// do runs an authenticated request and decodes a JSON answer into out.
func (e *env) do(method, path string, body any, out any) int {
	e.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	case []byte:
		rd = bytes.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("X-Internal-Key", testKey)
	if _, ok := body.([]byte); ok {
		req.Header.Set("Content-Type", "image/png")
	}
	rec := httptest.NewRecorder()
	e.api.ServeHTTP(rec, req)
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			e.t.Fatalf("%s %s: %d %q: %v", method, path, rec.Code, rec.Body.String(), err)
		}
	}
	return rec.Code
}

func (e *env) must(want int, method, path string, body any, out any) {
	e.t.Helper()
	var raw json.RawMessage
	if got := e.do(method, path, body, &raw); got != want {
		e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, got, want, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatal(err)
		}
	}
}

// console fetches a console route through the switch.
func (e *env) console(path string) (int, []byte) {
	rec := httptest.NewRecorder()
	e.sw.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Code, rec.Body.Bytes()
}

func (e *env) manifest(build int) *newsseed.Manifest {
	raw, err := os.ReadFile(filepath.Join(e.svc.buildDir(build), "seed", "manifest.json"))
	if err != nil {
		e.t.Fatal(err)
	}
	var m newsseed.Manifest
	json.Unmarshal(raw, &m)
	return &m
}

func manifestArticle(m *newsseed.Manifest, topic string, id int64) *newsseed.ManifestArticle {
	for _, ch := range m.Channels {
		for i := range ch.Articles {
			if ch.TopicID == topic && ch.Articles[i].NewsID == id {
				return &ch.Articles[i]
			}
		}
	}
	return nil
}

func pngBytes(t *testing.T, w, h int) []byte {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, im)
	return b.Bytes()
}

func TestEmbeddedPublicKeyMatchesPatchKey(t *testing.T) {
	committed, err := os.ReadFile("../../tools/news-patch/openpak-bcat-public.pem")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(committed, publicKeyPEM) {
		t.Fatal("internal/newsadmin/openpak-bcat-public.pem differs from tools/news-patch/openpak-bcat-public.pem")
	}
	if OpenPakPublicKey().N.BitLen() != 2048 {
		t.Fatal("not an RSA-2048 key")
	}
}

func TestAuth(t *testing.T) {
	e := newEnv(t, false)
	for _, tc := range []struct {
		key, header string
		want        int
	}{
		{"", "", 503}, {"", "anything", 503}, {testKey, "", 401}, {testKey, "wrong", 401}, {testKey, testKey, 200},
	} {
		req := httptest.NewRequest("GET", "/admin/v1/status", nil)
		if tc.header != "" {
			req.Header.Set("X-Internal-Key", tc.header)
		}
		rec := httptest.NewRecorder()
		e.svc.Handler(tc.key).ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("key %q header %q: %d, want %d", tc.key, tc.header, rec.Code, tc.want)
		}
		if tc.want != 200 && !strings.Contains(rec.Body.String(), `"error"`) {
			t.Errorf("error body missing: %s", rec.Body)
		}
	}
}

func TestImportAndCRUDValidation(t *testing.T) {
	e := newEnv(t, false)
	if _, err := os.Stat(filepath.Join(e.data, "channels", "screenshots")); err == nil {
		t.Fatal("screenshots imported without -import-screenshots")
	}
	var list struct{ Channels []channelView }
	e.must(200, "GET", "/admin/v1/channels", nil, &list)
	if len(list.Channels) != 1 || list.Channels[0].ArticleCount != 2 {
		t.Fatalf("imported channels: %+v", list.Channels)
	}

	// Channel validation.
	for _, body := range []string{
		`{"topic_id": "news", "name": "N", "publisher": "P", "description": "D"}`,
		`{"topic_id": "openpak_", "name": "N", "publisher": "P", "description": "D"}`,
		`{"topic_id": "openpak_x", "name": "", "publisher": "P", "description": "D"}`,
		`{"topic_id": "openpak_x", "name": "N", "publisher": "P", "description": "` + strings.Repeat("d", 121) + `"}`,
		`{"topic_id": "openpak_x", "name": "N", "publisher": "P", "description": "D", "icon": {"motif": "burst"}}`,
		`{"topic_id": "openpak_x", "name": "N", "publisher": "P", "description": "D", "application_ids": ["0100ABCD12340000"]}`,
		`{"topic_id": "openpak_x", "name": "N", "publisher": "P", "description": "D", "colour": "red"}`,
		`not json`,
	} {
		if got := e.do("POST", "/admin/v1/channels", body, nil); got != 400 {
			t.Errorf("create %s: %d, want 400", body, got)
		}
	}
	if got := e.do("POST", "/admin/v1/channels", `{"topic_id": "openpak_test", "name": "N", "publisher": "P", "description": "D"}`, nil); got != 409 {
		t.Errorf("duplicate topic: %d", got)
	}
	var ch channelView
	e.must(201, "POST", "/admin/v1/channels", map[string]any{"topic_id": "openpak_new", "name": "New", "publisher": "OpenPak",
		"description": "A new channel.", "shop_query": "dst_nsuid=1", "application_ids": []string{"0100abcd12340000"}}, &ch)
	if ch.Icon.Motif != "mark" || ch.ShopQuery != "dst_nsuid=1" || ch.Retired {
		t.Fatalf("created channel: %+v", ch)
	}
	e.must(200, "PATCH", "/admin/v1/channels/openpak_new", `{"shop_query": null, "description": "Changed.", "has_icon_image": true}`, &ch)
	if ch.ShopQuery != "" || ch.Description != "Changed." || ch.HasIconImage {
		t.Fatalf("patched channel: %+v", ch)
	}
	if got := e.do("PATCH", "/admin/v1/channels/openpak_new", `{"topic_id": "openpak_other"}`, nil); got != 400 {
		t.Errorf("topic rename: %d", got)
	}
	if got := e.do("GET", "/admin/v1/channels/openpak_nope", nil, nil); got != 404 {
		t.Errorf("unknown channel: %d", got)
	}

	// Article validation.
	base := `"title": "T", "body": "B", "language": "en-GB", "supported_languages": ["en-GB"], "lifetime_days": 7, "art": {"motif": "orbit"}`
	for _, body := range []string{
		`{` + strings.Replace(base, `"B"`, `"line\nbreak"`, 1) + `}`,
		`{` + strings.Replace(base, `"en-GB"]`, `"en-US"]`, 1) + `}`,
		`{` + strings.Replace(base, `7`, `31`, 1) + `}`,
		`{` + base + `, "priority": 50}`,
		`{` + base + `, "pickup_days": 8}`,
		`{` + base + `, "essential_pickup": {"pickup_days": 0}}`,
		`{` + base + `, "published_at": "2026-09-23T12:00:00+02:00"}`,
		`{` + base + `, "more": {"type": "browser", "url": "http://insecure"}}`,
		`{` + base + `, "more": {"type": "game"}}`,
		`{` + base + `, "subtitle": "x"}`,
		`{` + strings.Replace(base, `"orbit"`, `"nope"`, 1) + `}`,
	} {
		if got := e.do("POST", "/admin/v1/channels/openpak_test/articles", body, nil); got != 400 {
			t.Errorf("create article %s: %d, want 400", body, got)
		}
	}
	if got := e.do("POST", "/admin/v1/channels/openpak_test/articles", `{"news_id": 2609100001, `+base+`}`, nil); got != 409 {
		t.Errorf("duplicate news_id: %d", got)
	}
	var a articleView
	e.must(201, "POST", "/admin/v1/channels/openpak_new/articles", `{`+base+`, "priority": 1500,
		"essential_pickup": {"pickup_days": 2}, "pickup_days": 3,
		"more": {"type": "shop", "query": "dst_nsuid=70010000000001"}}`, &a)
	if a.NewsID != 2609230001 || a.Priority != 1500 || a.EssentialPickup.PriorityAfter != 1500 || a.PublishedAt != nil || a.EverPublished {
		t.Fatalf("created article: %+v", a)
	}
	e.must(200, "PATCH", "/admin/v1/channels/openpak_new/articles/2609230001", `{"title": "Retitled", "more": null}`, &a)
	if a.Title != "Retitled" || a.More != nil {
		t.Fatalf("patched article: %+v", a)
	}
	if got := e.do("PATCH", "/admin/v1/channels/openpak_new/articles/2609230001", `{"news_id": 1}`, nil); got != 400 {
		t.Errorf("news_id change: %d", got)
	}
	if got := e.do("PATCH", "/admin/v1/channels/openpak_new/articles/2609230001", `{"retired": true}`, nil); got != 400 {
		t.Errorf("retire without reason: %d", got)
	}

	// Images.
	if got := e.do("PUT", "/admin/v1/channels/openpak_new/articles/2609230001/image", []byte("GIF89a not an image"), nil); got != 415 {
		t.Errorf("gif upload: %d", got)
	}
	e.must(200, "PUT", "/admin/v1/channels/openpak_new/articles/2609230001/image", pngBytes(t, 400, 300), &a)
	if !a.HasImage {
		t.Fatal("has_image not set")
	}
	req := httptest.NewRequest("GET", "/admin/v1/channels/openpak_new/articles/2609230001/image", nil)
	req.Header.Set("X-Internal-Key", testKey)
	rec := httptest.NewRecorder()
	e.api.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("image get: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	e.must(200, "PUT", "/admin/v1/channels/openpak_new/icon", pngBytes(t, 64, 64), &ch)
	if !ch.HasIconImage {
		t.Fatal("has_icon_image not set")
	}

	// Never published: delete removes it outright.
	var del map[string]any
	e.must(200, "DELETE", "/admin/v1/channels/openpak_new/articles/2609230001", nil, &del)
	if del["deleted"] != true {
		t.Fatalf("delete: %v", del)
	}
	if got := e.do("GET", "/admin/v1/channels/openpak_new/articles/2609230001", nil, nil); got != 404 {
		t.Errorf("deleted article still there: %d", got)
	}
	var st statusView
	e.must(200, "GET", "/admin/v1/status", nil, &st)
	if !st.Dirty || st.SigningConfigured || st.Serving.Build != nil {
		t.Fatalf("status: %+v", st)
	}
	// Imported ids count as published: deleting one retires it.
	e.must(200, "DELETE", "/admin/v1/channels/openpak_test/articles/2609100002", nil, &del)
	if del["retired"] != true {
		t.Fatalf("delete of an imported article: %v", del)
	}
}

func TestPublishWithoutKeys(t *testing.T) {
	e := newEnv(t, false)
	if got := e.do("POST", "/admin/v1/publish", nil, nil); got != 503 {
		t.Fatalf("publish without keys: %d", got)
	}
	var st statusView
	e.must(200, "GET", "/admin/v1/status", nil, &st)
	if st.LastError == nil || st.LastError.Op != "publish" || strings.Contains(st.LastError.Error, "/") {
		t.Fatalf("last_error: %+v", st.LastError)
	}
}

func TestPublishVerifySwitchAndPush(t *testing.T) {
	e := newEnv(t, true)
	if code, _ := e.console("/api/nx/v1/list/openpak_test"); code != 503 {
		t.Fatalf("before publish, no tree: %d", code)
	}
	var res PublishResult
	e.must(200, "POST", "/admin/v1/publish", `{"note": "first"}`, &res)
	if res.Build != 1 || res.Revision != 1 || res.Channels != 1 || res.Articles != 2 || res.Push != "queued" ||
		strings.Join(res.ChangedTopics, ",") != "openpak_test" {
		t.Fatalf("publish: %+v", res)
	}

	// Every container verifies the way sign-seed.sh checks it.
	seed, signed := filepath.Join(e.svc.buildDir(1), "seed"), filepath.Join(e.svc.buildDir(1), "signed")
	n := 0
	filepath.WalkDir(seed, func(path string, d fs.DirEntry, err error) error {
		if !d.Type().IsRegular() || !(strings.HasSuffix(path, ".msgpack") || d.Name() == "icon.jpg") {
			return nil
		}
		rel, _ := filepath.Rel(seed, path)
		plain, _ := os.ReadFile(path)
		container, err := os.ReadFile(filepath.Join(signed, strings.TrimSuffix(strings.TrimSuffix(rel, ".msgpack"), ".jpg")+".bcat"))
		if err != nil {
			t.Fatalf("%s: no container: %v", rel, err)
		}
		if len(container) != 288+len(plain) {
			t.Errorf("%s: size %d, want 288+%d", rel, len(container), len(plain))
		}
		got, err := bcatcontainer.Verify(container, e.aesKey, &signingKey(t).PublicKey)
		if err != nil || !bytes.Equal(got, plain) {
			t.Errorf("%s: does not verify: %v", rel, err)
		}
		n++
		return nil
	})
	if n != res.Containers || n == 0 {
		t.Fatalf("verified %d containers, publish reported %d", n, res.Containers)
	}
	if _, err := os.Stat(filepath.Join(signed, "openpak_test", "2609100001", "local.msgpack")); err != nil {
		t.Fatal("local.msgpack missing")
	}
	if _, err := os.Stat(filepath.Join(signed, "SHA256SUMS")); err != nil {
		t.Fatal("SHA256SUMS missing")
	}

	// The console routes now serve build 1.
	code, list1 := e.console("/api/nx/v1/list/openpak_test")
	want, _ := os.ReadFile(filepath.Join(signed, "openpak_test", "list.bcat"))
	if code != 200 || !bytes.Equal(list1, want) {
		t.Fatalf("served list: %d", code)
	}
	e.svc.WaitPushes()
	if len(e.pushes.got) != 1 || e.pushes.got[0]["topic_id"] != "openpak_test" || e.pushes.got[0]["revision"] != float64(1) || e.pushes.keys[0] != "nx-key" {
		t.Fatalf("pushes: %v %v", e.pushes.got, e.pushes.keys)
	}

	// Publish stamps published_at, persistently.
	var a articleView
	e.must(200, "GET", "/admin/v1/channels/openpak_test/articles/2609100001", nil, &a)
	if a.PublishedAt == nil || *a.PublishedAt != "2026-09-23T12:00:00Z" || *a.ExpireAt != "2026-09-30T12:00:00Z" || !a.EverPublished {
		t.Fatalf("stamped article: %+v", a)
	}
	var st statusView
	e.must(200, "GET", "/admin/v1/status", nil, &st)
	if st.Dirty || *st.Serving.Build != 1 || st.LastPublish.Note != "first" || len(st.Pushes) != 1 || !st.Pushes[0].OK {
		t.Fatalf("status after publish: %+v", st)
	}

	// Nothing changed: a republish changes no list and pushes nothing.
	e.now = e.now.Add(time.Hour)
	e.must(200, "POST", "/admin/v1/publish", nil, &res)
	if res.Build != 2 || len(res.ChangedTopics) != 0 || res.Push != "none" {
		t.Fatalf("idle republish: %+v", res)
	}

	// An edit switches the served tree on the next publish.
	e.must(200, "PATCH", "/admin/v1/channels/openpak_test/articles/2609100001", `{"title": "First, edited"}`, nil)
	e.must(200, "GET", "/admin/v1/status", nil, &st)
	if !st.Dirty {
		t.Fatal("edit did not mark the source dirty")
	}
	e.must(200, "POST", "/admin/v1/publish", nil, &res)
	_, list3 := e.console("/api/nx/v1/list/openpak_test")
	if bytes.Equal(list1, list3) || res.Build != 3 || res.Revision != 3 {
		t.Fatalf("switch did not happen: %+v", res)
	}
	e.svc.WaitPushes()
	if last := e.pushes.got[len(e.pushes.got)-1]; last["revision"] != float64(3) {
		t.Fatalf("push revision: %v", last)
	}
}

func TestOverwriteContinuityAndRollback(t *testing.T) {
	e := newEnv(t, true)
	var res PublishResult
	e.must(200, "POST", "/admin/v1/publish", nil, &res) // build 1
	if a := manifestArticle(e.manifest(1), "openpak_test", 2609100001); a == nil || a.Overwrite {
		t.Fatalf("first build overwrite: %+v", a)
	}
	e.must(200, "PATCH", "/admin/v1/channels/openpak_test/articles/2609100001", `{"body": "Changed body."}`, nil)
	e.must(200, "POST", "/admin/v1/publish", nil, &res) // build 2
	if !manifestArticle(e.manifest(2), "openpak_test", 2609100001).Overwrite {
		t.Fatal("content change did not set overwrite")
	}
	if manifestArticle(e.manifest(2), "openpak_test", 2609100002).Overwrite {
		t.Fatal("unchanged article got overwrite")
	}

	// Moving published_at alone is not a content change.
	e.must(200, "PATCH", "/admin/v1/channels/openpak_test/articles/2609100002", `{"published_at": "2026-09-24T00:00:00Z"}`, nil)

	// Roll back to build 1: consoles get build 1 again.
	_, list2 := e.console("/api/nx/v1/list/openpak_test")
	e.must(200, "POST", "/admin/v1/rollback", nil, &res)
	_, served := e.console("/api/nx/v1/list/openpak_test")
	want, _ := os.ReadFile(filepath.Join(e.svc.buildDir(1), "signed", "openpak_test", "list.bcat"))
	if res.Build != 1 || !bytes.Equal(served, want) || bytes.Equal(served, list2) {
		t.Fatalf("rollback: %+v", res)
	}
	var st statusView
	e.must(200, "GET", "/admin/v1/status", nil, &st)
	if *st.Serving.Build != 1 || !st.Dirty {
		t.Fatalf("status after rollback: %+v", st)
	}
	if got := e.do("POST", "/admin/v1/rollback", nil, nil); got != 409 {
		t.Fatalf("second rollback with nothing earlier: %d", got)
	}
	if got := e.do("POST", "/admin/v1/rollback", `{"build": 99}`, nil); got != 409 {
		t.Fatalf("rollback to an unknown build: %d", got)
	}

	// Publishing after a rollback compares against the newest build (2),
	// so the sticky overwrite survives; the time-only move sets nothing.
	e.must(200, "POST", "/admin/v1/publish", nil, &res) // build 3
	m := e.manifest(3)
	if !manifestArticle(m, "openpak_test", 2609100001).Overwrite {
		t.Fatal("overwrite lost after rollback")
	}
	if a := manifestArticle(m, "openpak_test", 2609100002); a.Overwrite || a.PublishedAt != time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC).Unix() {
		t.Fatalf("published_at move: %+v", a)
	}

	// Rollback to an explicit build, then retention keeps 3 builds and the served one.
	e.must(200, "POST", "/admin/v1/rollback", `{"build": 2}`, &res)
	for i := 0; i < 3; i++ {
		e.must(200, "POST", "/admin/v1/publish", nil, &res)
	}
	e.must(200, "GET", "/admin/v1/status", nil, &st)
	if len(st.Builds) != 3 || st.Builds[0].Build != 6 {
		t.Fatalf("kept builds: %+v", st.Builds)
	}
	if _, err := os.Stat(e.svc.buildDir(1)); err == nil {
		t.Fatal("build 1 not pruned")
	}
	e.svc.WaitPushes()
}

func TestPublishFailureKeepsServing(t *testing.T) {
	e := newEnv(t, true)
	e.must(200, "POST", "/admin/v1/publish", nil, nil)
	_, before := e.console("/api/nx/v1/list/openpak_test")
	// A source file edited behind the API's back that fails to build.
	os.WriteFile(filepath.Join(e.data, "channels", "lineup.json"), []byte(`{"channels": [{"topic_id": "openpak_test"}]}`), 0644)
	if got := e.do("POST", "/admin/v1/publish", nil, nil); got != 422 {
		t.Fatalf("broken source: %d", got)
	}
	_, after := e.console("/api/nx/v1/list/openpak_test")
	if !bytes.Equal(before, after) {
		t.Fatal("a failed publish changed the served tree")
	}
	if _, err := os.Stat(e.svc.buildDir(2)); err == nil {
		t.Fatal("failed build directory left behind")
	}
	var st statusView
	e.must(200, "GET", "/admin/v1/status", nil, &st)
	if st.LastError == nil || *st.Serving.Build != 1 {
		t.Fatalf("status: %+v", st)
	}
}

func TestRetireChannelAndArticle(t *testing.T) {
	e := newEnv(t, true)
	e.must(200, "POST", "/admin/v1/publish", nil, nil)
	if got := e.do("PATCH", "/admin/v1/channels/openpak_test/articles/2609100002", `{"retired": true, "retire_reason": "superseded"}`, nil); got != 200 {
		t.Fatalf("retire article: %d", got)
	}
	if got := e.do("PATCH", "/admin/v1/channels/openpak_test/articles/2609100002", `{"title": "x"}`, nil); got != 409 {
		t.Fatalf("edit retired article: %d", got)
	}
	e.must(200, "POST", "/admin/v1/publish", nil, nil)
	if code, _ := e.console("/openpak/v1/topics/openpak_test/news/2609100002/expired"); code != 200 {
		t.Fatalf("expired replacement not served: %d", code)
	}

	var ch channelView
	e.must(200, "PATCH", "/admin/v1/channels/openpak_test", `{"retired": true, "retire_reason": "channel closed"}`, &ch)
	if !ch.Retired || ch.RetireReason != "channel closed" {
		t.Fatalf("retired channel: %+v", ch)
	}
	if got := e.do("POST", "/admin/v1/channels/openpak_test/articles", `{}`, nil); got != 409 {
		t.Fatalf("article on a retired channel: %d", got)
	}
	if got := e.do("PATCH", "/admin/v1/channels/openpak_test", `{"name": "x"}`, nil); got != 409 {
		t.Fatalf("edit retired channel: %d", got)
	}
	var res PublishResult
	e.must(200, "POST", "/admin/v1/publish", nil, &res)
	if code, _ := e.console("/api/nx/v1/list/openpak_test"); code != 200 {
		t.Fatalf("expired list of the retired channel not served: %d", code)
	}
	if code, _ := e.console("/api/nx/v1/topics/openpak_test/detail"); code == 200 {
		t.Fatal("retired channel still in the lineup routes")
	}
	if _, err := os.Stat(filepath.Join(e.svc.buildDir(3), "signed", "retired", "openpak_test", "list.bcat")); err != nil {
		t.Fatal("no expired list container")
	}

	// Restore: back in the lineup, articles stay retired.
	e.must(200, "PATCH", "/admin/v1/channels/openpak_test", `{"retired": false}`, &ch)
	if ch.Retired || ch.ActiveArticleCount != 0 {
		t.Fatalf("restored channel: %+v", ch)
	}
	e.must(200, "PATCH", "/admin/v1/channels/openpak_test/articles/2609100001", `{"retired": false}`, nil)
	e.must(200, "POST", "/admin/v1/publish", nil, nil)
	if code, _ := e.console("/openpak/v1/topics/openpak_test/news/2609100001/article"); code != 200 {
		t.Fatalf("restored article not served: %d", code)
	}
	e.svc.WaitPushes()
}

func TestRestartServesPublishedBuild(t *testing.T) {
	e := newEnv(t, true)
	e.must(200, "POST", "/admin/v1/publish", nil, nil)
	_, before := e.console("/api/nx/v1/list/openpak_test")
	sw := &newsexperiment.Switch{}
	cfg := e.svc.cfg
	cfg.Switch = sw
	os.MkdirAll(filepath.Join(e.data, "builds", "000099"), 0755) // crashed publish leftover
	if _, err := New(cfg); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	sw.ServeHTTP(rec, httptest.NewRequest("GET", "/api/nx/v1/list/openpak_test", nil))
	if !bytes.Equal(rec.Body.Bytes(), before) {
		t.Fatal("restart does not serve the published build")
	}
	if _, err := os.Stat(filepath.Join(e.data, "builds", "000099")); err == nil {
		t.Fatal("leftover build directory not cleaned")
	}
	e.svc.WaitPushes()
}
