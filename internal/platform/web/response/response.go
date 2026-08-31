package response

import (
	"encoding/json"
	"net/http"
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		InternalError(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
