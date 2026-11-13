// Origin: nodelogicau/particulars-cli internal/prov/prov.go @ e8933aacecad
// (environment variables renamed to EPISODES_*; the document fields and the
// synthesis harness rule dropped, since an episode cites nothing)

// Package prov resolves who recorded an episode the same way for every
// front-end: explicit values, then EPISODES_* environment variables, then
// defaults.source in episodes.yaml.
package prov

import (
	"os"
	"regexp"
	"strings"

	"github.com/nodelogicau/episodes-cli/internal/apperr"
	"github.com/nodelogicau/episodes-cli/internal/model"
)

// Environment variables for provenance defaults.
const (
	EnvAuthor  = "EPISODES_AUTHOR"
	EnvHarness = "EPISODES_HARNESS"
	EnvModel   = "EPISODES_MODEL"
)

// Explicit holds values supplied directly by a caller (flags or tool args).
type Explicit struct {
	Author, Harness, Model string
}

// placeholder matches an unsubstituted template variable such as
// "${user_config.author}". Hosts that expand a manifest leave these literal
// when the value is optional and unset, and a literal recorded as
// source.author is worse than no author at all, so it is treated as absent.
var placeholder = regexp.MustCompile(`^\$\{[^}]*\}$`)

// IsPlaceholder reports whether v is an unsubstituted template variable.
func IsPlaceholder(v string) bool { return placeholder.MatchString(strings.TrimSpace(v)) }

func first(vs ...string) string {
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" && !IsPlaceholder(v) {
			return v
		}
	}
	return ""
}

// Resolve layers explicit → environment → defaults, field by field.
func Resolve(defaults model.Source, e Explicit) model.Source {
	return model.Source{
		Author:  first(e.Author, os.Getenv(EnvAuthor), defaults.Author),
		Harness: first(e.Harness, os.Getenv(EnvHarness), defaults.Harness),
		Model:   first(e.Model, os.Getenv(EnvModel), defaults.Model),
	}
}

// Require enforces the format's minimum: at least one of author or harness.
func Require(src model.Source) error {
	if src.Author == "" && src.Harness == "" {
		return apperr.Refused("source: a source needs at least one of author or harness: pass --author/--harness, set %s/%s, or set defaults.source in episodes.yaml", EnvAuthor, EnvHarness)
	}
	return nil
}
