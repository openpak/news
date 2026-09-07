package switchbcat

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// etag derives a strong validator from response bytes (PRD NB-06).
func etag(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:16])
}

// serveConditional writes CDN-shaped caching headers and reports whether
// the body should still be written (false = 304 or HEAD already handled).
func serveConditional(w http.ResponseWriter, r *http.Request, etagv string, mod time.Time, size int64) bool {
	etagQ := `"` + etagv + `"`
	w.Header().Set("ETag", etagQ)
	w.Header().Set("Cache-Control", "public, max-age=300")
	if !mod.IsZero() {
		w.Header().Set("Last-Modified", mod.UTC().Format(http.TimeFormat))
	}
	if size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		if etagMatch(inm, etagv) {
			w.WriteHeader(http.StatusNotModified)
			return false
		}
	} else if ims := r.Header.Get("If-Modified-Since"); ims != "" && !mod.IsZero() {
		if t, err := http.ParseTime(ims); err == nil && !mod.Truncate(time.Second).After(t) {
			w.WriteHeader(http.StatusNotModified)
			return false
		}
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return false
	}
	return true
}

func etagMatch(header, etagv string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimPrefix(strings.TrimSpace(part), "W/")
		part = strings.Trim(part, `"`)
		if part == "*" || part == etagv {
			return true
		}
	}
	return false
}
