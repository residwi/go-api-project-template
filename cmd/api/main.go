package main

import (
	"os"

	apihttp "github.com/residwi/go-api-project-template/internal/transport/http"
)

func main() {
	if err := apihttp.Run(); err != nil {
		os.Exit(1)
	}
}
