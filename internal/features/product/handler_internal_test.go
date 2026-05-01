package product

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

func TestPublicHandler_GetBySlug_EmptySlug(t *testing.T) {
	h := &publicHandler{
		service:   &Service{},
		validator: validator.New(),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/products/", nil)

	h.GetBySlug(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp response.Response
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.False(t, resp.Success)
	assert.Equal(t, "slug is required", resp.Error.Message)
}
