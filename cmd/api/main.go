package main

import (
	"os"

	apihttp "github.com/residwi/go-api-project-template/internal/server"
)

func main() {
	if err := apihttp.Run(); err != nil {
		os.Exit(1)
	}
}
