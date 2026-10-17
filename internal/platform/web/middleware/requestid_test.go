package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

func TestRequestID_GeneratesUUIDv7WhenNoHeader(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	id, err := uuid.Parse(rec.Header().Get("X-Request-ID"))
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(7), id.Version())
}

func TestRequestID_UsesExistingHeader(t *testing.T) {
	existingID := "my-custom-request-id"

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", existingID)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, existingID, rec.Header().Get("X-Request-ID"))
}

func TestRequestID_DownstreamLogsCarryTheSameIDAsTheResponseHeader(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(logger.ContextHandler{Handler: slog.NewJSONHandler(&buf, nil)})

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.InfoContext(r.Context(), "downstream work")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "req-42")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	assert.Equal(t, "req-42", record["request_id"])
	assert.Equal(t, rec.Header().Get("X-Request-ID"), record["request_id"])
}
