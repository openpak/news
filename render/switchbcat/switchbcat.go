// Package switchbcat is the Switch channel renderer (PRD NB-08): it turns
// the platform-neutral article stream into the console's BCAT topics
// format, and serves the Switch-only per-title BCAT datasets (list, data,
// images) plus the emulator delivery (NB-05).
//
// The Switch adapter mounts Mux on the three bcat hostnames (contract
// request NB-1):
//
//	bcat-topics-lp1.cdn.nintendo.net  GET /api/nx/v1/titles/<title>/topics?c[]=FR&c[]=GB
//	bcat-list-lp1.cdn.nintendo.net    GET /v1/list
//	bcat-data-lp1.cdn.nintendo.net    GET /<title>/<path>, /images/<key>, manifest route
//
// The wire shapes live in topics.go and data.go, one file each, so the
// NB-00 spike (what the console actually accepts) can adjust them quickly.
package switchbcat

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openpak/news/internal/content"
	"github.com/openpak/news/render"
)

// Source is everything the Switch channel may read: the platform-neutral
// Reader (renderers never hold content of their own).
type Source interface {
	content.Reader
}

type Service struct {
	src           Source
	log           *slog.Logger
	publicBaseURL string
}

func NewService(src Source, publicBaseURL string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{src: src, publicBaseURL: strings.TrimRight(publicBaseURL, "/"), log: log}
}

// Renderer implements render.Renderer (NB-08): audience in, console-format
// out. The topics JSON body is the Switch News channel delivery format.
type Renderer struct{ svc *Service }

func (s *Service) Renderer() render.Renderer { return Renderer{s} }

func (r Renderer) Name() string { return "switch-bcat" }

func (r Renderer) Render(ctx context.Context, q content.AudienceQuery) (render.Payload, error) {
	q.Platform = "switch"
	articles, mod, err := r.svc.src.Articles(ctx, q)
	if err != nil {
		return render.Payload{}, err
	}
	body, err := topicsJSON(articles, q, r.svc.publicBaseURL)
	if err != nil {
		return render.Payload{}, err
	}
	return render.Payload{
		ContentType: "application/json",
		ETag:        etag(body),
		ModTime:     mod,
		Body:        body,
	}, nil
}

// Mux returns the handlers the adapter mounts on the three bcat hostnames.
// One mux serves all of them: the routes are path-distinct.
func (s *Service) Mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// topics host: per-title topics with country/language filters (NB-01).
	mux.HandleFunc("GET /api/nx/v1/titles/{title}/topics", s.handleTopics)

	// list host: titles with served datasets (NB-02).
	mux.HandleFunc("GET /v1/list", s.handleList)

	// data host: per-title dataset files, images, manifest (NB-02, NB-06).
	mux.HandleFunc("GET /images/{key}", s.handleImage)
	mux.HandleFunc("GET /api/nx/v1/titles/{title}/manifest", s.handleManifest)
	mux.HandleFunc("GET /{title}/{path...}", s.handleDataFile)

	// emulator delivery (NB-05): same datasets for Citron's LayeredFS.
	mux.HandleFunc("GET /api/emulator/v1/bcat/titles", s.handleEmuTitles)
	mux.HandleFunc("GET /api/emulator/v1/bcat/titles/{title}", s.handleEmuManifest)
	mux.HandleFunc("GET /api/emulator/v1/bcat/titles/{title}/files/{path...}", s.handleEmuFile)

	return mux
}

// ---------- handlers ----------

func (s *Service) handleTopics(w http.ResponseWriter, r *http.Request) {
	titleID := strings.ToLower(r.PathValue("title"))
	q := content.AudienceQuery{Platform: "switch", TitleID: titleID}
	qs := r.URL.Query()
	q.Regions = qs["c[]"]
	q.Language = first(qs, "l", "lang", "language", "l[]")

	// The console teaches the schema (NB-00): log every topics request
	// with its raw query so the spike can compare served vs accepted.
	s.log.Info("bcat topics request", "title", titleID, "query", r.URL.RawQuery,
		"regions", q.Regions, "language", q.Language)

	payload, err := s.Renderer().Render(r.Context(), q)
	if err != nil {
		s.log.Error("topics render failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.serveBytes(w, r, payload.Body, payload.ContentType, payload.ETag, payload.ModTime)
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	titles, mod, err := s.src.Titles(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	body, err := listJSON(titles)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.serveBytes(w, r, body, "application/json", etag(body), mod)
}

func (s *Service) handleManifest(w http.ResponseWriter, r *http.Request) {
	titleID := strings.ToLower(r.PathValue("title"))
	ds, mod, err := s.src.CurrentDataset(r.Context(), titleID)
	if err != nil {
		s.dataErr(w, err)
		return
	}
	body, err := manifestJSON(titleID, ds, "")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.serveBytes(w, r, body, "application/json", etag(body), mod)
}

func (s *Service) handleDataFile(w http.ResponseWriter, r *http.Request) {
	titleID := strings.ToLower(r.PathValue("title"))
	path := r.PathValue("path")
	if content.ValidateFilePath(path) != nil {
		http.NotFound(w, r)
		return
	}
	ds, _, err := s.src.CurrentDataset(r.Context(), titleID)
	if err != nil {
		s.dataErr(w, err)
		return
	}
	s.serveDatasetFile(w, r, ds, path)
}

func (s *Service) handleImage(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	rc, size, mod, err := s.src.OpenObject(r.Context(), key)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", contentTypeOf(key))
	if !serveConditional(w, r, etag([]byte(key)), mod, size) {
		return
	}
	http.ServeContent(w, r, "", mod, rc)
}

// ---------- emulator delivery (NB-05) ----------

func (s *Service) handleEmuTitles(w http.ResponseWriter, r *http.Request) {
	titles, mod, err := s.src.Titles(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	body, err := emulatorTitlesJSON(titles)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.serveBytes(w, r, body, "application/json", etag(body), mod)
}

func (s *Service) handleEmuManifest(w http.ResponseWriter, r *http.Request) {
	titleID := strings.ToLower(r.PathValue("title"))
	ds, mod, err := s.src.CurrentDataset(r.Context(), titleID)
	if err != nil {
		s.dataErr(w, err)
		return
	}
	body, err := manifestJSON(titleID, ds, "/api/emulator/v1/bcat/titles/"+titleID+"/files/")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.serveBytes(w, r, body, "application/json", etag(body), mod)
}

func (s *Service) handleEmuFile(w http.ResponseWriter, r *http.Request) {
	titleID := strings.ToLower(r.PathValue("title"))
	path := r.PathValue("path")
	if content.ValidateFilePath(path) != nil {
		http.NotFound(w, r)
		return
	}
	ds, _, err := s.src.CurrentDataset(r.Context(), titleID)
	if err != nil {
		s.dataErr(w, err)
		return
	}
	s.serveDatasetFile(w, r, ds, path)
}

// ---------- helpers ----------

func (s *Service) serveDatasetFile(w http.ResponseWriter, r *http.Request, ds *content.Dataset, path string) {
	for _, f := range ds.Files {
		if f.Path != path {
			continue
		}
		rc, size, mod, err := s.src.OpenObject(r.Context(), f.Key)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Content-SHA256", f.SHA256)
		if !serveConditional(w, r, f.SHA256, mod, size) {
			return
		}
		http.ServeContent(w, r, "", mod, rc)
		return
	}
	http.NotFound(w, r)
}

func (s *Service) serveBytes(w http.ResponseWriter, r *http.Request, body []byte, ctype, etagv string, mod time.Time) {
	w.Header().Set("Content-Type", ctype)
	if !serveConditional(w, r, etagv, mod, int64(len(body))) {
		return
	}
	w.Write(body)
}

func (s *Service) dataErr(w http.ResponseWriter, err error) {
	if errors.Is(err, content.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func first(q url.Values, keys ...string) string {
	for _, k := range keys {
		if v := q.Get(k); v != "" {
			return v
		}
	}
	return ""
}

func contentTypeOf(name string) string {
	switch {
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".webp"):
		return "image/webp"
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return "image/jpeg"
	default:
		return "application/octet-stream"
	}
}
