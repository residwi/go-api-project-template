package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	promotionhttp "github.com/residwi/go-api-project-template/internal/modules/promotion/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	promoMocks "github.com/residwi/go-api-project-template/mocks/promotion"
)

func setupPromotionMux(t *testing.T) (*http.ServeMux, *promoMocks.MockRepository) {
	repo := promoMocks.NewMockRepository(t)
	svc := promotion.NewService(repo, testhelper.FakeTxRunner{})
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	promotionhttp.RegisterRoutes(authed, admin, promotionhttp.RouteDeps{
		Validator: v,
		Service:   svc,
	})

	return mux, repo
}

func setPromoAuthContext(r *http.Request) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}

func TestHandler_Apply_ServiceError(t *testing.T) {
	t.Run("service returns not found", func(t *testing.T) {
		mux, repo := setupPromotionMux(t)

		repo.EXPECT().GetByCode(mock.Anything, "NOTEXIST").Return(nil, apperror.ErrNotFound)

		body, _ := json.Marshal(map[string]any{
			"code":     "NOTEXIST",
			"subtotal": 5000,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/promotions/apply", bytes.NewReader(body))
		r = setPromoAuthContext(r)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("service returns bad request for inactive promo", func(t *testing.T) {
		mux, repo := setupPromotionMux(t)

		repo.EXPECT().GetByCode(mock.Anything, "INACTIVE").Return(&promotion.Promotion{
			ID:        uuid.New(),
			Code:      "INACTIVE",
			Active:    false,
			StartsAt:  time.Now().Add(-time.Hour),
			ExpiresAt: time.Now().Add(time.Hour),
		}, nil)

		body, _ := json.Marshal(map[string]any{
			"code":     "INACTIVE",
			"subtotal": 5000,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/promotions/apply", bytes.NewReader(body))
		r = setPromoAuthContext(r)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_Apply_Success(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo := setupPromotionMux(t)

		repo.EXPECT().GetByCode(mock.Anything, "SAVE10").Return(&promotion.Promotion{
			ID:             uuid.New(),
			Code:           "SAVE10",
			Type:           promotion.TypeFixedAmount,
			Value:          1000,
			MinOrderAmount: 500,
			Active:         true,
			StartsAt:       time.Now().Add(-time.Hour),
			ExpiresAt:      time.Now().Add(time.Hour),
		}, nil)

		body, _ := json.Marshal(map[string]any{
			"code":     "SAVE10",
			"subtotal": 5000,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/promotions/apply", bytes.NewReader(body))
		r = setPromoAuthContext(r)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})
}
