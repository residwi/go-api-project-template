package response_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core/response"
)

type sampleReq struct {
	Name string `json:"name"`
}

type fakeValidator struct {
	errs map[string]any
}

func (f fakeValidator) Validate(_ any) map[string]any {
	return f.errs
}

func TestBind(t *testing.T) {
	t.Run("returns the decoded request when valid", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"widget"}`))
		w := httptest.NewRecorder()

		got, ok := response.Bind[sampleReq](w, r, fakeValidator{errs: nil})

		require.True(t, ok)
		assert.Equal(t, sampleReq{Name: "widget"}, got)
		assert.Equal(t, http.StatusOK, w.Code) // nothing written on success
	})

	t.Run("rejects malformed JSON with 400", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{not json`))
		w := httptest.NewRecorder()

		_, ok := response.Bind[sampleReq](w, r, fakeValidator{errs: nil})

		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("rejects invalid fields with 422", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":""}`))
		w := httptest.NewRecorder()

		_, ok := response.Bind[sampleReq](w, r, fakeValidator{errs: map[string]any{"name": "is required"}})

		assert.False(t, ok)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})
}
