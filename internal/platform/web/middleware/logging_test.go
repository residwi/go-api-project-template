package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

func TestLogging_CallsNextHandler(t *testing.T) {
	called := false
	handler := Logging(testLogger())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestLogging_StatusRecorderRecordsStatusCode(t *testing.T) {
	handler := Logging(testLogger())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestLoggingNamesTheResolvedClientIP(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(logger.ContextHandler{Handler: slog.NewJSONHandler(&buf, nil)})

	handler := ClientIP(nil)(Logging(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "198.51.100.9")
	handler.ServeHTTP(httptest.NewRecorder(), r)

	assert.Contains(t, buf.String(), `"client_ip":"198.51.100.9"`)
	assert.NotContains(t, buf.String(), "remote_addr")
}
