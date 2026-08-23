package main

import (
	"os"

	"github.com/residwi/go-api-project-template/internal/server"
)

func main() {
	if err := server.Run(); err != nil {
		os.Exit(1)
	}
}
