package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

func Setup(level, format string) *slog.Logger {
	return setup(os.Stdout, level, format)
}

func setup(w io.Writer, level, format string) *slog.Logger {
	if strings.EqualFold(level, "warning") {
		level = "warn"
	}

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
