package main

import (
	"log"

	apihttp "github.com/residwi/go-api-project-template/internal/transport/http"
)

func main() {
	// Stdlib log, not slog: apihttp.Run is what builds the application logger,
	// and the errors reported here are the ones that happen before or instead of
	// that.
	if err := apihttp.Run(); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
}
