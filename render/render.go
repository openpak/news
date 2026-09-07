// Package render is the renderer contract (PRD NB-08): audience in,
// console-format out. Every console channel is one implementation; content
// stays platform-neutral in the content package and renderers never hold
// content of their own.
package render

import (
	"context"
	"time"

	"github.com/openpak/news/internal/content"
)

// Payload is one rendered response for one audience query.
type Payload struct {
	ContentType string
	ETag        string // strong validator, no quotes
	ModTime     time.Time
	Body        []byte
}

// Renderer renders the platform-neutral article stream for one audience
// into a channel's native delivery format. The Switch BCAT renderer
// (render/switchbcat) is the first implementation; a SpotPass renderer
// implements the same interface later without touching content code.
type Renderer interface {
	// Name identifies the channel, e.g. "switch-bcat".
	Name() string
	// Render converts the articles visible to the audience into the
	// channel's format.
	Render(ctx context.Context, q content.AudienceQuery) (Payload, error)
}

// Source is what any renderer is allowed to know about content: the read
// side only (PRD NB-07: renderers never hold content of their own).
type Source interface {
	Articles(ctx context.Context, q content.AudienceQuery) ([]content.Article, time.Time, error)
}
