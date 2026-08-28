package web

import "log/slog"

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
