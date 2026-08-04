package response

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
)

func TestOK(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	OK(w, map[string]string{"key": "value"})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.Success)
}

func TestCreated(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	Created(w, map[string]string{"id": "123"})

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestNoContent(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	NoContent(w)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestPaginated(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	data := map[string]any{"items": []string{"a", "b"}, "total": 2}
	Paginated(w, data)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.Success)
}

func TestErr(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	Err(w, http.StatusBadRequest, "bad request", map[string]any{"field": "email"})

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.False(t, body.Success)
	assert.Equal(t, &Error{
		Message: "bad request",
		Details: map[string]any{"field": "email"},
	}, body.Error)
}

func TestBadRequest(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	BadRequest(w, "invalid input")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestNotFound(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	NotFound(w, "resource not found")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUnauthorized(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	Unauthorized(w, "invalid token")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestForbidden(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	Forbidden(w, "admin only")
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestConflict(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	Conflict(w, "already exists")
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestTooManyRequests(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	TooManyRequests(w, "rate limit exceeded")
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestInternalError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	InternalError(w)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestValidationErr(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	ValidationErr(w, map[string]any{"email": "required"})
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	var body Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, &Error{
		Message: "validation failed",
		Details: map[string]any{"email": "required"},
	}, body.Error)
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()

	t.Run("valid JSON", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"test"}`))
		var dst struct {
			Name string `json:"name"`
		}
		err := DecodeJSON(w, r, &dst)
		require.NoError(t, err)
		assert.Equal(t, "test", dst.Name)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{invalid`))
		var dst struct{}
		err := DecodeJSON(w, r, &dst)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("body too large", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		bigBody := `{"name":"` + strings.Repeat("a", (1<<20)+1) + `"}`
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(bigBody))
		var dst struct {
			Name string `json:"name"`
		}
		err := DecodeJSON(w, r, &dst)
		require.Error(t, err)
		require.ErrorIs(t, err, apperror.ErrBadRequest)
		assert.Contains(t, err.Error(), "request body too large")
	})
}

func TestAppError_Error(t *testing.T) {
	t.Parallel()

	t.Run("returns message when Err is nil", func(t *testing.T) {
		t.Parallel()

		appErr := NewAppError(http.StatusBadRequest, "something wrong", nil)
		assert.Equal(t, "something wrong", appErr.Error())
	})

	t.Run("returns wrapped error string when Err is set", func(t *testing.T) {
		t.Parallel()

		underlying := errors.New("db connection failed")
		appErr := NewAppError(http.StatusInternalServerError, "something wrong", underlying)
		assert.Equal(t, "db connection failed", appErr.Error())
	})
}

func TestAppError_Unwrap(t *testing.T) {
	t.Parallel()

	underlying := errors.New("original cause")
	appErr := NewAppError(http.StatusInternalServerError, "wrapped", underlying)
	assert.Equal(t, underlying, appErr.Unwrap())
}

func TestNewAppErrorWithDetails(t *testing.T) {
	t.Parallel()

	details := map[string]any{"field": "email", "reason": "invalid format"}
	appErr := NewAppErrorWithDetails(http.StatusUnprocessableEntity, "validation failed", details, nil)

	assert.Equal(t, &AppError{
		Status:  http.StatusUnprocessableEntity,
		Message: "validation failed",
		Details: details,
	}, appErr)
}

func TestHandleErr(t *testing.T) {
	t.Parallel()

	t.Run("app error", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		err := NewAppError(http.StatusTeapot, "I'm a teapot", nil)
		HandleErr(w, err)
		assert.Equal(t, http.StatusTeapot, w.Code)
	})

	t.Run("app error with details", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		details := map[string]any{"key": "val"}
		err := NewAppErrorWithDetails(http.StatusBadRequest, "bad", details, nil)
		HandleErr(w, err)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		var body Response
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "val", body.Error.Details["key"])
	})

	t.Run("ErrNotFound returns 404", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrNotFound)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("ErrConflict returns 409", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrConflict)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("ErrBadRequest returns 400", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrBadRequest)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ErrUnauthorized returns 401", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrUnauthorized)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("ErrForbidden returns 403", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrForbidden)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("ErrInvalidCredentials returns 401", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrInvalidCredentials)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("ErrTokenExpired returns 401", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrTokenExpired)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("ErrInvalidToken returns 401", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrInvalidToken)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("ErrInsufficientStock returns 409", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrInsufficientStock)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("ErrCartEmpty returns 400", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrCartEmpty)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ErrOrderNotPayable returns 400", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrOrderNotPayable)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ErrOrderCharging returns 409", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrOrderCharging)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("ErrAmountMismatch returns 409", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrAmountMismatch)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("ErrCouponExhausted returns 409", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrCouponExhausted)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("ErrFulfillmentFailed returns 409", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, apperror.ErrFulfillmentFailed)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("wrapped sentinel error maps correctly", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		wrapped := fmt.Errorf("%w: user with email already exists", apperror.ErrBadRequest)
		HandleErr(w, wrapped)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		var body Response
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Contains(t, body.Error.Message, "user with email already exists")
	})

	t.Run("unknown error returns 500", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		HandleErr(w, errors.New("unknown"))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
