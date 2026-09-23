// SPDX-License-Identifier: AGPL-3.0-only
package newsseed

import (
	"image"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// Source file types, exported for the CLI and the admin API. The JSON
// shapes are the channels/ file format.
type (
	Channel         = channelSource
	Lineup          = lineupSource
	Article         = articleSource
	Art             = artSpec
	More            = moreSpec
	EssentialPickup = essentialSpec
	Retired         = retiredFile
	RetiredEntry    = retiredEntry
	Manifest        = manifest
	ManifestArticle = articleEntry
	FileInfo        = fileInfo
)

// Options tune a build.
type Options struct {
	// Adopt migrates a pre-marker output directory (see --adopt).
	Adopt bool
	// AllowEmptyChannels lets a lineup channel have no articles yet (an
	// empty archive and list). The file-based CLI keeps rejecting it as a
	// likely typo; the admin API creates channels before their articles.
	AllowEmptyChannels bool
}

// Build loads the lineup and retirement list from source and builds the
// seed into out, returning the manifest it wrote.
func Build(source, out string, published time.Time, opts Options) (*Manifest, error) {
	l, err := loadLineup(source)
	if err != nil {
		return nil, err
	}
	r, err := loadRetired(filepath.Join(source, "retired.json"))
	if err != nil {
		return nil, err
	}
	return buildOpts(source, out, published, l, r, opts)
}

// LoadLineup reads and validates <source>/lineup.json.
func LoadLineup(source string) (*Lineup, error) { return loadLineup(source) }

// LoadRetired reads <source>/retired.json (absent = empty).
func LoadRetired(path string) (*Retired, error) { return loadRetired(path) }

// Save writes the retirement list sorted, as the CLI does.
func (r *retiredFile) Save(path string) error { return r.save(path) }

// Has reports whether topic/id is retired.
func (r *retiredFile) Has(topic string, id int64) bool { return r.has(topic, id) }

// LoadArticle reads and validates one <topic>/<news_id>.json file.
func LoadArticle(path string) (*Article, error) { return loadArticle(path) }

// Validate checks an article's fields.
func (a *articleSource) Validate() error { return a.validate() }

// Validate checks a channel block's fields.
func (ch *channelSource) Validate() error { return ch.validate() }

// PrepareOutDir seeds a fresh output directory with a previous build's
// manifest (and the newsseed marker), so the build computes overwrite
// flags against it exactly as a rebuild into the same directory would.
func PrepareOutDir(out string, prevManifest []byte) error {
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	if prevManifest == nil {
		return nil
	}
	if err := os.WriteFile(filepath.Join(out, markerName), markerContent, 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "manifest.json"), prevManifest, 0644)
}

// IconMotifs and SceneMotifs list the valid icon.motif and art.motif values.
func IconMotifs() []string { return []string{"mark", "mark-gear", "mark-pad", "mark-question"} }

func SceneMotifs() []string {
	out := make([]string, 0, len(sceneMotifs))
	for name := range sceneMotifs {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ImageExts are the accepted uploaded-image extensions, in lookup order.
var ImageExts = []string{".png", ".jpg", ".jpeg"}

// ScreenshotPath and IconPath name uploaded images inside a source dir
// (without extension).
func ScreenshotPath(source string, id int64) string {
	return filepath.Join(source, "screenshots", strconv.FormatInt(id, 10))
}

func IconPath(source, topic string) string { return filepath.Join(source, "icons", topic) }

// channelIcon renders the channel's 70x70 icon: an uploaded
// icons/<topic>.png|jpg when present, else the motif.
func channelIcon(source string, ch channelSource) ([]byte, error) {
	if im := loadImage(IconPath(source, ch.TopicID)); im != nil {
		return encodeJPEG(coverCrop(im, 70, 70))
	}
	return renderJPEG("icon", ch.Icon.Motif, ch.Icon.Hue, 70, 70)
}

func loadImage(base string) image.Image {
	for _, ext := range ImageExts {
		f, err := os.Open(base + ext)
		if err != nil {
			continue
		}
		im, _, err := image.Decode(f)
		f.Close()
		if err == nil {
			return im
		}
	}
	return nil
}
