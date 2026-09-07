// Package switchbcat is the Switch channel (PRD NB-08). After the NB-00
// finding (docs/spike-nb00-review.md) the console-facing bcat surfaces are
// NO-GO: the topics/list/data payloads on a stock console are signed,
// encrypted bcat content containers we cannot produce. This package is
// therefore **emulator-only**: it serves the per-title BCAT datasets to
// Citron's LayeredFS flow (NB-05) and implements render.Renderer by
// rendering the delivery manifest for a title audience.
//
// Console reachability is handled outside this repo: the setup homebrew
// places dataset files on the SD (LayeredFS), driven by the same manifest
// contract — see README "LayeredFS placement contract" (N2').
package switchbcat

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
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

// Renderer implements render.Renderer (NB-08): audience in, channel
// format out. The Switch channel's delivery format is the BCAT dataset
// manifest for the queried title (consumed by the emulator and by the
// setup homebrew; never by a stock console).
type Renderer struct{ svc *Service }

func (s *Service) Renderer() render.Renderer { return Renderer{s} }

func (r Renderer) Name() string { return "switch-bcat" }

func (r Renderer) Render(ctx context.Context, q content.AudienceQuery) (render.Payload, error) {
	q.Platform = "switch"
	if q.TitleID == "" {
		return render.Payload{}, errors.New("switch-bcat renderer needs a title id")
	}
	ds, mod, err := r.svc.src.CurrentDataset(ctx, q.TitleID)
	if err != nil {
		return render.Payload{}, err
	}
	body, err := manifestJSON(q.TitleID, ds, r.svc.publicBaseURL+"/api/emulator/v1/bcat/titles/"+q.TitleID+"/files/")
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

// Mux returns the emulator-delivery handlers (NB-05). The adapter mounts
// them on its emulator API (ports.md 20070); the standalone news HTTP
// server serves the same routes for development. There are deliberately
// no bcat-hostname console routes left.
func (s *Service) Mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Article images: referenced by the read API (NB-07) with absolute
	// URLs on this host; host-neutral, consumed by website/apps/emulator.
	mux.HandleFunc("GET /images/{key}", s.handleImage)

	// Emulator delivery (NB-05): per-title BCAT datasets for Citron's
	// LayeredFS flow.
	mux.HandleFunc("GET /api/emulator/v1/bcat/titles", s.handleEmuTitles)
	mux.HandleFunc("GET /api/emulator/v1/bcat/titles/{title}", s.handleEmuManifest)
	mux.HandleFunc("GET /api/emulator/v1/bcat/titles/{title}/files/{path...}", s.handleEmuFile)

	return mux
}

// ---------- handlers ----------

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
