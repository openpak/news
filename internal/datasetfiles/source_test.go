package datasetfiles

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openpak/news/render/switchbcat"
)

const testTitle = "0100f8f0000a2000"

func fixture(t *testing.T, mutate func(map[string]any)) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, testTitle)
	if err := os.MkdirAll(filepath.Join(dir, "files", "vsdata"), 0755); err != nil {
		t.Fatal(err)
	}
	data := []byte("test payload")
	hash := sha256.Sum256(data)
	m := map[string]any{
		"format": 1, "title_id": testTitle,
		"valid_from":  time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		"valid_until": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"files":       []map[string]any{{"path": "vsdata/VSSetting_0.byaml", "size": len(data), "sha256": hex.EncodeToString(hash[:])}},
	}
	if mutate != nil {
		mutate(m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{
		filepath.Join(dir, "manifest.json"):                        b,
		filepath.Join(dir, "files", "vsdata", "VSSetting_0.byaml"): data,
	} {
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func get(handler http.Handler, path string, etag string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if etag != "" {
		r.Header.Set("If-None-Match", etag)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestRendererDeliveryUsesValidatedSnapshot(t *testing.T) {
	root := fixture(t, nil)
	source, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	h := switchbcat.NewService(source, "", nil).Mux()
	index := get(h, "/api/emulator/v1/bcat/titles", "")
	if index.Code != 200 || !strings.Contains(index.Body.String(), testTitle) {
		t.Fatalf("index: %d %s", index.Code, index.Body)
	}
	path := "/api/emulator/v1/bcat/titles/" + testTitle
	m := get(h, path, "")
	if m.Code != 200 {
		t.Fatalf("manifest: %d %s", m.Code, m.Body)
	}
	var manifest struct {
		Files []struct{ URL, SHA256 string }
	}
	if err := json.Unmarshal(m.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 1 || manifest.Files[0].URL != path+"/files/vsdata/VSSetting_0.byaml" {
		t.Fatalf("file URLs: %+v", manifest.Files)
	}
	// A change underneath the snapshot must not make the manifest lie.
	if err := os.WriteFile(filepath.Join(root, testTitle, "files", "vsdata", "VSSetting_0.byaml"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	f := get(h, manifest.Files[0].URL, "")
	if f.Code != 200 || f.Body.String() != "test payload" || f.Header().Get("X-Content-SHA256") != manifest.Files[0].SHA256 {
		t.Fatalf("file: %d %s", f.Code, f.Body)
	}
	if r := get(h, manifest.Files[0].URL, f.Header().Get("ETag")); r.Code != 304 || r.Body.Len() != 0 {
		t.Fatalf("conditional GET: %d %s", r.Code, r.Body)
	}
	if r := get(h, path+"/files/vsdata/unlisted.byaml", ""); r.Code != 404 {
		t.Fatalf("unlisted file: %d", r.Code)
	}
}

func TestRejectInvalidManifestAndBytes(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"title":     func(m map[string]any) { m["title_id"] = "0100000000000000" },
		"unbounded": func(m map[string]any) { m["valid_until"] = nil },
		"empty":     func(m map[string]any) { m["files"] = []any{} },
		"hash":      func(m map[string]any) { m["files"].([]map[string]any)[0]["sha256"] = strings.Repeat("0", 64) },
		"size":      func(m map[string]any) { m["files"].([]map[string]any)[0]["size"] = 1 },
		"traversal": func(m map[string]any) { m["files"].([]map[string]any)[0]["path"] = "../VSSetting_0.byaml" },
		"deep":      func(m map[string]any) { m["files"].([]map[string]any)[0]["path"] = "a/b/c.byaml" },
		"duplicate": func(m map[string]any) { files := m["files"].([]map[string]any); m["files"] = append(files, files[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(fixture(t, mutate)); err == nil {
				t.Fatal("accepted invalid dataset")
			}
		})
	}
}

func TestExpiredDatasetIsNotPublished(t *testing.T) {
	source, err := Load(fixture(t, func(m map[string]any) {
		m["valid_until"] = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	}))
	if err != nil {
		t.Fatal(err)
	}
	h := switchbcat.NewService(source, "", nil).Mux()
	if r := get(h, "/api/emulator/v1/bcat/titles", ""); r.Code != 200 || r.Body.String() != `{"titles":[]}` {
		t.Fatalf("index: %d %s", r.Code, r.Body)
	}
	for _, path := range []string{"", "/files/vsdata/VSSetting_0.byaml"} {
		if r := get(h, "/api/emulator/v1/bcat/titles/"+testTitle+path, ""); r.Code != 404 {
			t.Fatalf("expired data: %d", r.Code)
		}
	}
}

func TestPayloadCannotEscapeDatasetRoot(t *testing.T) {
	root := fixture(t, nil)
	outside := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(outside, []byte("test payload"), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, testTitle, "files", "vsdata", "VSSetting_0.byaml")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("accepted a payload symlink outside the dataset root")
	}
}
