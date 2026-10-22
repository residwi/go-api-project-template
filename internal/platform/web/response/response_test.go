package response

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestOK_Envelope(t *testing.T) {
	t.Parallel()

	t.Run("keeps data for an empty list", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		OK(w, []int{})

		assert.JSONEq(t, `{"success":true,"data":[]}`, w.Body.String())
	})

	t.Run("encodes a nil list as an empty array", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		OK(w, []int(nil))

		assert.JSONEq(t, `{"success":true,"data":[]}`, w.Body.String())
	})

	t.Run("omits data when there is none", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		OK(w, nil)

		assert.JSONEq(t, `{"success":true}`, w.Body.String())
	})

	t.Run("writes map keys in sorted order so equal data gives equal bytes", func(t *testing.T) {
		t.Parallel()

		data := map[string]int{}
		var want strings.Builder
		want.WriteString(`{"success":true,"data":{`)
		for i, key := range strings.Split("abcdefghijklmnopqrstuvwxyz", "") {
			data[key] = i
			if i > 0 {
				want.WriteString(",")
			}
			fmt.Fprintf(&want, "%q:%d", key, i)
		}
		want.WriteString("}}")

		w := httptest.NewRecorder()
		OK(w, data)

		assert.Equal(t, want.String(), w.Body.String())
	})
}

func TestOK_UnencodableValueBecomesA500(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	// A channel cannot be marshalled, and json.Marshal buffers the whole document, so
	// this fails before anything reaches the wire: the 200 OK() asked for must not be
	// what the client sees.
	OK(w, make(chan int))

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"an unencodable value must not be reported to the client as success")
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body),
		"the fallback body must still be valid JSON")
	assert.False(t, body.Success)
	require.NotNil(t, body.Error)
	assert.Equal(t, "internal server error", body.Error.Message)
	assert.Nil(t, body.Data)
}

func TestErr_UnencodableDetailsStillTerminate(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	// An unencodable value in Details sends writeJSON back through InternalError,
	// which re-enters with none: it must settle after one bounce, not loop.
	Err(w, http.StatusBadRequest, "bad request", map[string]any{"ch": make(chan int)})

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var body Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.False(t, body.Success)
	require.NotNil(t, body.Error)
	assert.Equal(t, "internal server error", body.Error.Message)
	assert.Nil(t, body.Error.Details)
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
