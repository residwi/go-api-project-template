package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/middleware"
)

func TestRequestID_GeneratesUUIDWhenNoHeader(t *testing.T) {
	var capturedID string
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = middleware.GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.NotEmpty(t, capturedID)
	assert.Len(t, capturedID, 36) // UUID format: 8-4-4-4-12
	assert.Equal(t, capturedID, rec.Header().Get("X-Request-ID"))
}

func TestRequestID_UsesExistingHeader(t *testing.T) {
	existingID := "my-custom-request-id"

	var capturedID string
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = middleware.GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", existingID)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, existingID, capturedID)
	assert.Equal(t, existingID, rec.Header().Get("X-Request-ID"))
}

func TestRequestID_SetsResponseHeader(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	responseID := rec.Header().Get("X-Request-ID")
	assert.NotEmpty(t, responseID)
}

func TestGetRequestID_ReturnsEmptyStringFromEmptyContext(t *testing.T) {
	id := middleware.GetRequestID(context.Background())

	assert.Empty(t, id)
}
