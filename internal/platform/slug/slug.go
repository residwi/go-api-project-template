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

// MakeOrFallback exists because a name with no ASCII alphanumerics (non-Latin
// script, symbols only) slugifies to "" and would collide on the NOT NULL UNIQUE
// slug column. Callers pass a unique fallback, e.g. a UUID-derived value.
func MakeOrFallback(name, fallback string) string {
	if slug := Make(name); slug != "" {
		return slug
	}
	return Make(fallback)
}
