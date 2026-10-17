package slug

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
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

func MakeOrFallback(name, fallback string) string {
	if slug := Make(name); slug != "" {
		return slug
	}
	return Make(fallback)
}

// Seed takes the tail of id, not its head: a v7 UUID starts with its
// timestamp, so the first 8 hex characters repeat for ~65s and two rows made
// close together would share a fallback slug.
func Seed(id uuid.UUID) string {
	s := id.String()
	return s[len(s)-8:]
}
