// Command mockgateway fakes a payment gateway so the API and worker can run
// locally without a real Stripe or Midtrans account.
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
	appLog := slog.New(slog.NewTextHandler(os.Stdout, nil))
	appLog.Info("mock payment gateway listening", slog.String("addr", addr))
	if err := http.ListenAndServe(addr, newMux(appLog)); err != nil { //nolint:gosec // dev-only tool
		appLog.Error("mock gateway failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func newMux(appLog *slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()
	mockserver.RegisterRoutes(mux, appLog)
	return mux
}
