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
// is allowed to take.
func Setup(level, format string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

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
