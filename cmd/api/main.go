package main

import (
	"log/slog"
	"os"

	"github.com/residwi/go-api-project-template/internal/server"
)

func main() {
	if err := server.Run(); err != nil {
		slog.Error("server failed to start", "error", err)
		os.Exit(1)
	}
}
