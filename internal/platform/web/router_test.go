package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func TestRouterGroup(t *testing.T) {
	t.Parallel()

	t.Run("nested groups compose their prefixes", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		router := NewRouter(mux, slog.New(slog.DiscardHandler))
		admin := router.Group("/api").Group("/admin")
		admin.Handle("GET /users", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/users", nil))

		assert.Equal(t, http.StatusTeapot, w.Code)
	})

	t.Run("a derived group runs the parent middleware outside its own", func(t *testing.T) {
		t.Parallel()

		var order []string
		mark := func(name string) Middleware {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					order = append(order, name)
					next.ServeHTTP(w, r)
				})
			}
		}

		mux := http.NewServeMux()
		parent := NewRouter(mux, slog.New(slog.DiscardHandler)).Group("/api", mark("parent"))
		child := parent.Group("/admin", mark("child"))
		child.Handle("GET /x", http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			order = append(order, "handler")
		}))

		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/admin/x", nil))

		assert.Equal(t, []string{"parent", "child", "handler"}, order)
	})

	t.Run("sibling groups do not inherit each other's middleware", func(t *testing.T) {
		t.Parallel()

		var seen []string
		mark := func(name string) Middleware {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					seen = append(seen, name)
					next.ServeHTTP(w, r)
				})
			}
		}

		mux := http.NewServeMux()
		router := NewRouter(mux, slog.New(slog.DiscardHandler))
		left := router.Group("/left", mark("left"))
		right := router.Group("/right", mark("right"))
		left.Handle("GET /x", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		right.Handle("GET /x", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/right/x", nil))

		assert.Equal(t, []string{"right"}, seen)
	})

	t.Run("the root group applies no middleware", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		router := NewRouter(mux, slog.New(slog.DiscardHandler))
		router.Handle("GET /health", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestRouterHandleFunc(t *testing.T) {
	t.Parallel()

	t.Run("a nil return leaves the handler's response alone and logs nothing", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		mux := http.NewServeMux()
		NewRouter(mux, captureLogger(&logs)).HandleFunc("GET /x", func(w http.ResponseWriter, _ *http.Request) error {
			response.OK(w, map[string]string{"ok": "yes"})
			return nil
		})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"success":true,"data":{"ok":"yes"}}`, rec.Body.String())
		assert.Empty(t, logs.String())
	})

	t.Run("a kind error answers its status and logs nothing", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		mux := http.NewServeMux()
		NewRouter(mux, captureLogger(&logs)).HandleFunc("GET /x", func(http.ResponseWriter, *http.Request) error {
			return fmt.Errorf("%w: order already paid", errs.ErrConflict)
		})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.JSONEq(t, `{"success":false,"error":{"message":"conflict: order already paid"}}`, rec.Body.String())
		assert.Empty(t, logs.String())
	})

	t.Run("an error with no kind answers the generic 500 and logs the cause", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		mux := http.NewServeMux()
		NewRouter(mux, captureLogger(&logs)).HandleFunc("GET /x", func(http.ResponseWriter, *http.Request) error {
			return errors.New("dial tcp 10.0.0.5:5432: connection refused")
		})

		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req = req.WithContext(logger.WithAttrs(req.Context(), slog.String("request_id", "req-1")))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.JSONEq(t, `{"success":false,"error":{"message":"internal server error"}}`, rec.Body.String())
		assert.Equal(t, []map[string]any{{
			"level":      "ERROR",
			"msg":        "unexpected error",
			"error":      "dial tcp 10.0.0.5:5432: connection refused",
			"request_id": "req-1",
		}}, decodeRecords(t, &logs))
	})

	t.Run("an error after the response was written keeps that response and logs once", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		mux := http.NewServeMux()
		NewRouter(mux, captureLogger(&logs)).HandleFunc("POST /x", func(w http.ResponseWriter, _ *http.Request) error {
			response.Created(w, map[string]string{"id": "1"})
			return errors.New("audit write failed")
		})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", nil))

		assert.Equal(t, http.StatusCreated, rec.Code)
		assert.JSONEq(t, `{"success":true,"data":{"id":"1"}}`, rec.Body.String())
		assert.Equal(t, []map[string]any{{
			"level": "ERROR",
			"msg":   "error after response was written",
			"error": "audit write failed",
		}}, decodeRecords(t, &logs))
	})

	t.Run("a kind error after the response was written is logged too", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		mux := http.NewServeMux()
		NewRouter(mux, captureLogger(&logs)).HandleFunc("POST /x", func(w http.ResponseWriter, _ *http.Request) error {
			response.NoContent(w)
			return fmt.Errorf("%w: cart is empty", errs.ErrBadRequest)
		})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", nil))

		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Empty(t, rec.Body.String())
		assert.Equal(t, []map[string]any{{
			"level": "ERROR",
			"msg":   "error after response was written",
			"error": "bad request: cart is empty",
		}}, decodeRecords(t, &logs))
	})

	t.Run("a validation error answers 422 with its details and logs nothing", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		mux := http.NewServeMux()
		NewRouter(mux, captureLogger(&logs)).HandleFunc("POST /x", func(http.ResponseWriter, *http.Request) error {
			return &response.ValidationError{Details: map[string]any{"email": "is required"}}
		})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", nil))

		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		assert.JSONEq(t,
			`{"success":false,"error":{"message":"validation failed","details":{"email":"is required"}}}`,
			rec.Body.String())
		assert.Empty(t, logs.String())
	})

	t.Run("group middleware wraps an error-returning handler", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		router := NewRouter(mux, slog.New(slog.DiscardHandler))
		router.Group("/api", func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Group", "api")
				next.ServeHTTP(w, r)
			})
		}).HandleFunc("GET /x", func(w http.ResponseWriter, _ *http.Request) error {
			response.NoContent(w)
			return nil
		})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/x", nil))

		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Equal(t, "api", rec.Header().Get("X-Group"))
	})
}

func captureLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(logger.ContextHandler{Handler: slog.NewJSONHandler(buf, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})})
}

func decodeRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any
	for line := range bytes.Lines(buf.Bytes()) {
		var record map[string]any
		require.NoError(t, json.Unmarshal(line, &record))
		records = append(records, record)
	}
	return records
}
