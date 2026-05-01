package main

import (
	"log"

	"github.com/residwi/go-api-project-template/internal/server"
)

func main() {
	if err := server.Run(); err != nil {
		log.Fatal(err)
	}
}
