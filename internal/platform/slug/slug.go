package slug

import (
	"regexp"
	"strings"
)

var (
	nonAlphanumeric = regexp.MustCompile(`[^a-z0-9-]+`)
	multiHyphen     = regexp.MustCompile(`-{2,}`)
)

func Make(s string) string {
	slug := strings.ToLower(s)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = nonAlphanumeric.ReplaceAllString(slug, "")
	slug = multiHyphen.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

// MakeOrFallback slugifies name, returning the (also slugified) fallback when
// the result would be empty. Names with no ASCII alphanumerics (non-Latin
// scripts, symbol-only names) slugify to "", which would collide on a NOT NULL
// UNIQUE slug column; callers pass a unique fallback (e.g. a UUID-derived value)
// so two such names never produce the same empty slug.
func MakeOrFallback(name, fallback string) string {
	if slug := Make(name); slug != "" {
		return slug
	}
	return Make(fallback)
}
