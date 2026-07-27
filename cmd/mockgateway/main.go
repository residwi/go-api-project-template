// Command mockgateway is a dev-only HTTP server that fakes a payment gateway
// (charge, refund, webhook trigger) so the API and worker can be run locally
// without a real Stripe/Midtrans account.
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/residwi/go-api-project-template/cmd/mockgateway/mockserver"
)

func main() {
	addr := os.Getenv("MOCK_GATEWAY_ADDR")
	if addr == "" {
		addr = ":9090"
	}
	slog.Info("mock payment gateway listening", "addr", addr)
	if err := http.ListenAndServe(addr, newMux()); err != nil { //nolint:gosec // dev-only tool
		slog.Error("mock gateway failed", "error", err)
		os.Exit(1)
	}
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mockserver.RegisterRoutes(mux)
	return mux
}
