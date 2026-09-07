package switchbcat

import (
	"encoding/json"
	"time"

	"github.com/openpak/news/internal/content"
)

// Wire shapes for the emulator delivery (NB-05). NOTE (NB-00): the stock
// console formats (bcat content containers, routes /api/nx/v1/...) are
// NO-GO for us — signed with Nintendo's key; see docs/spike-nb00-review.md.
// These JSON shapes are the OpenPak emulator/placement contract only. The
// console's documented list route (/api/nx/v1/list/<name>) is recorded as
// evidence and deliberately not served.

type manifestResponse struct {
	Format     int           `json:"format"`
	TitleID    string        `json:"title_id"`
	ValidFrom  string        `json:"valid_from"`
	ValidUntil *string       `json:"valid_until"`
	Files      []manifestDir `json:"files"`
}

type manifestDir struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
}

// manifestJSON renders the current dataset. fileURLPrefix empty means
// host-relative data URLs ("/<title>/<path>"); otherwise absolute
// emulator URLs.
func manifestJSON(titleID string, ds *content.Dataset, fileURLPrefix string) ([]byte, error) {
	m := manifestResponse{
		Format:    1,
		TitleID:   titleID,
		ValidFrom: ds.ValidFrom.UTC().Format(time.RFC3339),
		Files:     []manifestDir{},
	}
	if ds.ValidUntil != nil {
		vu := ds.ValidUntil.UTC().Format(time.RFC3339)
		m.ValidUntil = &vu
	}
	for _, f := range ds.Files {
		url := "/" + titleID + "/" + f.Path
		if fileURLPrefix != "" {
			url = fileURLPrefix + f.Path
		}
		m.Files = append(m.Files, manifestDir{Path: f.Path, Size: f.Size, SHA256: f.SHA256, URL: url})
	}
	return json.Marshal(m)
}

type emulatorTitlesResponse struct {
	Titles []string `json:"titles"`
}

func emulatorTitlesJSON(titles []string) ([]byte, error) {
	if titles == nil {
		titles = []string{}
	}
	return json.Marshal(emulatorTitlesResponse{Titles: titles})
}
