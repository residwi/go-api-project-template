package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Setup deliberately does not call [slog.SetDefault]: sloglint's no-global rule
// makes the package-level default unusable, so installing it would only offer a
// second way to log that nothing may take. Callers take a logger parameter.
func Setup(level, format string) *slog.Logger {
	return setup(os.Stdout, level, format)
}

// setup exists so the tests can read what was written.
func setup(w io.Writer, level, format string) *slog.Logger {
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
		handler = slog.NewTextHandler(w, opts)
	default:
		handler = slog.NewJSONHandler(w, opts)
	}

	return slog.New(ContextHandler{Handler: handler})
}
