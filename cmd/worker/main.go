package main

import (
	"os"

	"github.com/residwi/go-api-project-template/internal/worker"
)

func main() {
	if err := worker.Run(); err != nil {
		os.Exit(1)
	}
}
