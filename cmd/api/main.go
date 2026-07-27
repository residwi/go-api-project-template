package main

import (
	"log/slog"
	"os"

	apihttp "github.com/residwi/go-api-project-template/internal/transport/http"
)

func main() {
	if err := apihttp.Run(); err != nil {
		slog.Error("server failed to start", "error", err)
		os.Exit(1)
	}
}
