package core

import (
	"regexp"
	"strings"
)

var (
	nonAlphanumeric = regexp.MustCompile(`[^a-z0-9-]+`)
	multiHyphen     = regexp.MustCompile(`-{2,}`)
)

func Slugify(s string) string {
	slug := strings.ToLower(s)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = nonAlphanumeric.ReplaceAllString(slug, "")
	slug = multiHyphen.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

// SlugifyOrFallback slugifies name, returning the (also slugified) fallback when
// the result would be empty. Names with no ASCII alphanumerics (non-Latin
// scripts, symbol-only names) slugify to "", which would collide on a NOT NULL
// UNIQUE slug column; callers pass a unique fallback (e.g. a UUID-derived value)
// so two such names never produce the same empty slug.
func SlugifyOrFallback(name, fallback string) string {
	if slug := Slugify(name); slug != "" {
		return slug
	}
	return Slugify(fallback)
}
