package response

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/core"
)

type Response struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type Error struct {
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func OK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, Response{Success: true, Data: data})
}

func Created(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusCreated, Response{Success: true, Data: data})
}

func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func Paginated(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, Response{Success: true, Data: data})
}

func Err(w http.ResponseWriter, status int, message string, details map[string]any) {
	writeJSON(w, status, Response{
		Success: false,
		Error:   &Error{Message: message, Details: details},
	})
}

func BadRequest(w http.ResponseWriter, message string) {
	Err(w, http.StatusBadRequest, message, nil)
}

func NotFound(w http.ResponseWriter, message string) {
	Err(w, http.StatusNotFound, message, nil)
}

func Unauthorized(w http.ResponseWriter, message string) {
	Err(w, http.StatusUnauthorized, message, nil)
}

func Forbidden(w http.ResponseWriter, message string) {
	Err(w, http.StatusForbidden, message, nil)
}

func Conflict(w http.ResponseWriter, message string) {
	Err(w, http.StatusConflict, message, nil)
}

func TooManyRequests(w http.ResponseWriter, message string) {
	Err(w, http.StatusTooManyRequests, message, nil)
}

func InternalError(w http.ResponseWriter) {
	Err(w, http.StatusInternalServerError, "internal server error", nil)
}

func ValidationErr(w http.ResponseWriter, details map[string]any) {
	Err(w, http.StatusUnprocessableEntity, "validation failed", details)
}

// ParseUUIDParam parses a named URL path parameter as a UUID.
// On failure it writes a 400 response and returns false.
func ParseUUIDParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		BadRequest(w, "invalid "+name)
		return uuid.Nil, false
	}
	return id, true
}

func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB cap
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return fmt.Errorf("%w: request body too large", core.ErrBadRequest)
		}
		return fmt.Errorf("%w: %s", core.ErrBadRequest, err.Error())
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write response", "error", err)
	}
}
