package response

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

var (
	errTestBadRequest = fmt.Errorf("%w: test bad request", errs.ErrBadRequest)
	errTestConflict   = fmt.Errorf("%w: test conflict", errs.ErrConflict)
)

func TestHandleErr(t *testing.T) {
	t.Parallel()

	t.Run("wrapped sentinel maps through to its kind's status", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		HandleErr(rec, errTestBadRequest)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("conflict-kind wrapped sentinel maps to 409", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		HandleErr(rec, errTestConflict)

		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("further-wrapped error still maps through", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		HandleErr(rec, fmt.Errorf("context: %w", errTestConflict))

		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("generic sentinel maps directly", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		HandleErr(rec, errs.ErrForbidden)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("not found sentinel maps to 404", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		HandleErr(rec, errs.ErrNotFound)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("unauthorized sentinel maps to 401", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		HandleErr(rec, errs.ErrUnauthorized)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("matched row passes the wrapped message through", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		HandleErr(rec, errTestBadRequest)

		var body Response
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Contains(t, body.Error.Message, "test bad request")
	})

	t.Run("unknown error is 500 with a fixed body", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		HandleErr(rec, fmt.Errorf("some infrastructure failure"))

		require.Equal(t, http.StatusInternalServerError, rec.Code)

		var body Response
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, Response{
			Success: false,
			Error:   &Error{Message: "internal server error"},
		}, body)
	})
}
