package cart

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type stubRepo struct {
	getOrCreateID uuid.UUID
}

func (s *stubRepo) GetOrCreate(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	return s.getOrCreateID, nil
}
func (s *stubRepo) GetCart(context.Context, uuid.UUID) (*Cart, error) { return nil, nil } //nolint:nilnil // test stub
func (s *stubRepo) AddItem(_ context.Context, _, _ uuid.UUID, _ int) error {
	return nil
}

func (s *stubRepo) UpdateItemQuantity(_ context.Context, _, _ uuid.UUID, _ int) error {
	return nil
}
func (s *stubRepo) RemoveItem(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s *stubRepo) Clear(context.Context, uuid.UUID) error                 { return nil }
func (s *stubRepo) CountItems(context.Context, uuid.UUID) (int, error)     { return 0, nil }
func (s *stubRepo) GetCartForLock(context.Context, uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, nil
}

type stubProducts struct{}

func (s *stubProducts) GetByID(_ context.Context, id uuid.UUID) (*ProductInfo, error) {
	return &ProductInfo{ID: id, Name: "Widget", Price: 1000, Currency: "USD", Status: "published"}, nil
}

type stubStock struct{}

func (s *stubStock) GetStock(context.Context, uuid.UUID) (StockInfo, error) {
	return StockInfo{Available: 10}, nil
}

func newTestHandler() *handler {
	return &handler{
		service:   &Service{},
		validator: validator.New(),
	}
}

func setAuthContext(r *http.Request) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}

func TestHandler_GetCart(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/cart", nil)
		w := httptest.NewRecorder()

		h.GetCart(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.False(t, success)
	})
}

func TestHandler_AddItem(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/cart/items", nil)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/cart/items", strings.NewReader("{bad"))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/cart/items", strings.NewReader(`{}`))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.False(t, success)
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "validation failed", errBody["message"])
	})

	t.Run("success", func(t *testing.T) {
		repo := &stubRepo{getOrCreateID: uuid.New()}
		svc := NewService(repo, nil, &stubProducts{}, &stubStock{}, 50)
		h := &handler{service: svc, validator: validator.New()}

		userID := uuid.New()
		productID := uuid.New()

		body := fmt.Sprintf(`{"product_id":"%s","quantity":2}`, productID)
		r := httptest.NewRequest(http.MethodPost, "/cart/items", strings.NewReader(body))
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID, Email: "test@example.com", Role: "user",
		})
		r = r.WithContext(ctx)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)
	})
}

func TestHandler_UpdateItem(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPut, "/cart/items/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()

		h.UpdateItem(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid product UUID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPut, "/cart/items/bad", nil)
		r = setAuthContext(r)
		r.SetPathValue("product_id", "bad")
		w := httptest.NewRecorder()

		h.UpdateItem(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "invalid product_id")
	})

	t.Run("validation error missing quantity", func(t *testing.T) {
		productID := uuid.NewString()
		r := httptest.NewRequest(http.MethodPut, "/cart/items/"+productID, strings.NewReader(`{}`))
		r = setAuthContext(r)
		r.SetPathValue("product_id", productID)
		w := httptest.NewRecorder()

		h.UpdateItem(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		h := newTestHandler()
		productID := uuid.NewString()
		r := httptest.NewRequest(http.MethodPut, "/cart/items/"+productID, strings.NewReader("{bad"))
		r = setAuthContext(r)
		r.SetPathValue("product_id", productID)
		w := httptest.NewRecorder()

		h.UpdateItem(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		repo := &stubRepo{getOrCreateID: uuid.New()}
		svc := NewService(repo, nil, nil, nil, 50)
		h := &handler{service: svc, validator: validator.New()}

		userID := uuid.New()
		productID := uuid.New()

		r := httptest.NewRequest(http.MethodPut, "/cart/items/"+productID.String(), strings.NewReader(`{"quantity":5}`))
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID, Email: "test@example.com", Role: "user",
		})
		r = r.WithContext(ctx)
		r.SetPathValue("product_id", productID.String())
		w := httptest.NewRecorder()

		h.UpdateItem(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandler_RemoveItem(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodDelete, "/cart/items/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()

		h.RemoveItem(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid product UUID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodDelete, "/cart/items/bad", nil)
		r = setAuthContext(r)
		r.SetPathValue("product_id", "bad")
		w := httptest.NewRecorder()

		h.RemoveItem(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_Clear(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodDelete, "/cart", nil)
		w := httptest.NewRecorder()

		h.Clear(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
