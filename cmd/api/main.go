package main

import (
	"log"

	apihttp "github.com/residwi/go-api-project-template/internal/transport/http"
)

func main() {
	// Stdlib log, not slog: apihttp.Run builds the application logger, so nothing here
	// can reach it. This is the only report for a failure before the logger exists.
	// Anything after it is also recorded through the configured handler.
	if err := apihttp.Run(); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
}
