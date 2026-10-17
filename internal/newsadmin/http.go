// SPDX-License-Identifier: AGPL-3.0-only
package newsadmin

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/openpak/news/internal/newsseed"
)

const maxJSONBody = 256 << 10

// Handler serves docs/admin-api.md. An empty key disables every route
// (503); a wrong or missing X-Internal-Key answers 401.
func (s *Service) Handler(key string) http.Handler {
	mux := http.NewServeMux()
	route := func(pattern string, fn func(w http.ResponseWriter, r *http.Request) (any, int, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			v, status, err := fn(w, r)
			if err != nil {
				writeErr(w, err)
				return
			}
			if v == nil {
				return // fn wrote the response itself
			}
			writeJSONResponse(w, status, v)
		})
	}
	locked := func(fn func(r *http.Request) (any, int, error)) func(http.ResponseWriter, *http.Request) (any, int, error) {
		return func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			return fn(r)
		}
	}
	withBody := func(fn func(r *http.Request, body map[string]json.RawMessage) (any, int, error)) func(http.ResponseWriter, *http.Request) (any, int, error) {
		return func(w http.ResponseWriter, r *http.Request) (any, int, error) {
			body, err := readJSON(w, r)
			if err != nil {
				return nil, 0, err
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			return fn(r, body)
		}
	}

	route("GET /admin/v1/status", locked(func(r *http.Request) (any, int, error) { return s.status(), 200, nil }))
	route("GET /admin/v1/bcat/current", locked(func(r *http.Request) (any, int, error) { return s.bcatCurrent(), 200, nil }))
	route("GET /admin/v1/bcat/builds/{n}/seed.tar", func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return nil, 0, s.writeSeedTar(w, r.PathValue("n"))
	})
	route("GET /admin/v1/motifs", func(http.ResponseWriter, *http.Request) (any, int, error) {
		return map[string][]string{"icon": newsseed.IconMotifs(), "scene": newsseed.SceneMotifs()}, 200, nil
	})

	route("GET /admin/v1/channels", locked(func(r *http.Request) (any, int, error) {
		v, err := s.listChannels()
		return map[string]any{"channels": v}, 200, err
	}))
	route("POST /admin/v1/channels", withBody(func(r *http.Request, b map[string]json.RawMessage) (any, int, error) {
		v, err := s.createChannel(b)
		return v, 201, err
	}))
	route("GET /admin/v1/channels/{topic}", locked(func(r *http.Request) (any, int, error) {
		v, err := s.getChannel(r.PathValue("topic"))
		return v, 200, err
	}))
	route("PATCH /admin/v1/channels/{topic}", withBody(func(r *http.Request, b map[string]json.RawMessage) (any, int, error) {
		v, err := s.updateChannel(r.PathValue("topic"), b)
		return v, 200, err
	}))

	route("GET /admin/v1/channels/{topic}/articles", locked(func(r *http.Request) (any, int, error) {
		v, err := s.listArticles(r.PathValue("topic"))
		return map[string]any{"articles": v}, 200, err
	}))
	route("POST /admin/v1/channels/{topic}/articles", withBody(func(r *http.Request, b map[string]json.RawMessage) (any, int, error) {
		v, err := s.createArticle(r.PathValue("topic"), b)
		return v, 201, err
	}))
	route("GET /admin/v1/channels/{topic}/articles/{id}", locked(func(r *http.Request) (any, int, error) {
		v, err := s.getArticle(r.PathValue("topic"), r.PathValue("id"))
		return v, 200, err
	}))
	route("PATCH /admin/v1/channels/{topic}/articles/{id}", withBody(func(r *http.Request, b map[string]json.RawMessage) (any, int, error) {
		v, err := s.updateArticle(r.PathValue("topic"), r.PathValue("id"), b)
		return v, 200, err
	}))
	route("DELETE /admin/v1/channels/{topic}/articles/{id}", locked(func(r *http.Request) (any, int, error) {
		v, err := s.deleteArticle(r.PathValue("topic"), r.PathValue("id"))
		return v, 200, err
	}))

	// Images: the channel icon (id "") and the article image.
	for _, p := range []struct{ path, id string }{
		{"/admin/v1/channels/{topic}/icon", ""},
		{"/admin/v1/channels/{topic}/articles/{id}/image", "id"},
	} {
		idOf := func(r *http.Request) string {
			if p.id == "" {
				return ""
			}
			return r.PathValue("id")
		}
		view := func(r *http.Request) (any, error) {
			if p.id == "" {
				return s.getChannel(r.PathValue("topic"))
			}
			return s.getArticle(r.PathValue("topic"), r.PathValue("id"))
		}
		route("GET "+p.path, func(w http.ResponseWriter, r *http.Request) (any, int, error) {
			s.mu.Lock()
			base, err := s.imageTarget(r.PathValue("topic"), idOf(r), false)
			var data []byte
			var ct string
			if err == nil {
				data, ct, err = readImage(base)
			}
			s.mu.Unlock()
			if err != nil {
				return nil, 0, err
			}
			w.Header().Set("Content-Type", ct)
			w.Header().Set("Cache-Control", "no-store")
			w.Write(data)
			return nil, 0, nil
		})
		route("PUT "+p.path, func(w http.ResponseWriter, r *http.Request) (any, int, error) {
			data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImageBytes))
			if err != nil {
				return nil, 0, errf(http.StatusRequestEntityTooLarge, "image larger than %d bytes", maxImageBytes)
			}
			if ct := r.Header.Get("Content-Type"); ct != "" && ct != "image/png" && ct != "image/jpeg" && ct != "application/octet-stream" {
				return nil, 0, errf(http.StatusUnsupportedMediaType, "image must be PNG or JPEG")
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			base, err := s.imageTarget(r.PathValue("topic"), idOf(r), true)
			if err != nil {
				return nil, 0, err
			}
			if err := s.putImage(base, data); err != nil {
				return nil, 0, err
			}
			v, err := view(r)
			return v, 200, err
		})
		route("DELETE "+p.path, locked(func(r *http.Request) (any, int, error) {
			base, err := s.imageTarget(r.PathValue("topic"), idOf(r), true)
			if err != nil {
				return nil, 0, err
			}
			if imageExists(base) {
				removeImage(base)
				if err := s.touch(); err != nil {
					return nil, 0, err
				}
			}
			v, err := view(r)
			return v, 200, err
		}))
	}

	route("POST /admin/v1/publish", func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		body, err := readJSONOptional(w, r)
		if err != nil {
			return nil, 0, err
		}
		var in struct {
			Note string `json:"note"`
		}
		if err := decodeStrict(body, &in); err != nil {
			return nil, 0, err
		}
		if len(in.Note) > maxReason {
			return nil, 0, errf(http.StatusBadRequest, "note must be at most %d bytes", maxReason)
		}
		v, err := s.Publish(strings.TrimSpace(in.Note))
		return v, 200, err
	})
	route("POST /admin/v1/rollback", func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		body, err := readJSONOptional(w, r)
		if err != nil {
			return nil, 0, err
		}
		var in struct {
			Build int `json:"build"`
		}
		if err := decodeStrict(body, &in); err != nil {
			return nil, 0, err
		}
		v, err := s.Rollback(in.Build)
		return v, 200, err
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, errf(http.StatusNotFound, "no route %s %s", r.Method, r.URL.Path))
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key == "" {
			writeErr(w, errf(http.StatusServiceUnavailable, "admin API disabled: NEWS_ADMIN_KEY is not set"))
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Internal-Key")), []byte(key)) != 1 {
			writeErr(w, errf(http.StatusUnauthorized, "missing or wrong X-Internal-Key"))
			return
		}
		mux.ServeHTTP(w, r)
	})
}

type statusView struct {
	Serving struct {
		Build *int      `json:"build"`
		Since time.Time `json:"since"`
	} `json:"serving"`
	Revision          int64        `json:"revision"`
	Dirty             bool         `json:"dirty"`
	SigningConfigured bool         `json:"signing_configured"`
	PushConfigured    bool         `json:"push_configured"`
	LastPublish       *publishInfo `json:"last_publish"`
	LastError         *errInfo     `json:"last_error"`
	Pushes            []PushResult `json:"pushes"`
	Builds            []buildInfo  `json:"builds"`
}

func (s *Service) status() statusView {
	var v statusView
	if n := s.state.Serving; n != 0 {
		v.Serving.Build = &n
		v.Dirty = s.state.BuildRevs[n] != s.state.SourceRev
	} else {
		v.Dirty = s.state.SourceRev != 0
	}
	v.Serving.Since = s.state.ServingSince
	v.Revision = s.state.Revision
	v.SigningConfigured = s.keysConfigured() || s.cfg.Bcat != nil
	v.PushConfigured = s.cfg.Bcat != nil || (s.cfg.Push != nil && s.cfg.Push.URL != "")
	v.LastPublish, v.LastError = s.state.LastPublish, s.state.LastError
	v.Pushes = append([]PushResult{}, s.state.Pushes...)
	v.Builds = []buildInfo{}
	for i := len(s.state.Builds) - 1; i >= 0; i-- {
		v.Builds = append(v.Builds, s.state.Builds[i])
	}
	return v
}

func readJSON(w http.ResponseWriter, r *http.Request) (map[string]json.RawMessage, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxJSONBody))
	if err != nil {
		return nil, errf(http.StatusRequestEntityTooLarge, "body larger than %d bytes", maxJSONBody)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return nil, errf(http.StatusBadRequest, "body must be a JSON object")
	}
	return m, nil
}

func readJSONOptional(w http.ResponseWriter, r *http.Request) (map[string]json.RawMessage, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxJSONBody))
	if err != nil {
		return nil, errf(http.StatusRequestEntityTooLarge, "body larger than %d bytes", maxJSONBody)
	}
	if strings.TrimSpace(string(raw)) == "" {
		return map[string]json.RawMessage{}, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return nil, errf(http.StatusBadRequest, "body must be a JSON object")
	}
	return m, nil
}

func decodeStrict(m map[string]json.RawMessage, out any) error {
	return mergeInto(map[string]any{}, m, nil, out)
}

func writeErr(w http.ResponseWriter, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		writeJSONResponse(w, ae.status, map[string]string{"error": ae.msg})
		return
	}
	writeJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func writeJSONResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}
