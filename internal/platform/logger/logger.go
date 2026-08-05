package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Setup builds the application logger and returns it. It deliberately does not
// call [slog.SetDefault]: the returned logger is passed explicitly to whatever
// needs one, and sloglint's no-global rule makes the package-level default
// unusable, so installing it would only offer a second way to log that nothing
// is allowed to take. Callers therefore always take the logger as a parameter.
func Setup(level, format string) *slog.Logger {
	if strings.EqualFold(level, "warning") {
		level = "warn" // UnmarshalText only knows the short form.
	}

	// UnmarshalText leaves lvl untouched when it cannot parse, so an unknown
	// level stays Info. It also accepts offsets such as "warn+1" for free.
	lvl := slog.LevelInfo
	_ = lvl.UnmarshalText([]byte(level))

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
