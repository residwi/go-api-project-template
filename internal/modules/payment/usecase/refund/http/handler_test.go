package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_Refund(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupRefundMux(t)

		paymentID := uuid.New()

		cmd.EXPECT().Execute(mock.Anything, paymentID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/admin/payments/"+paymentID.String()+"/refund", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			Status string `json:"status"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			Status string `json:"status"`
		}{Status: "refund_enqueued"}, got)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupRefundMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/admin/payments/not-a-uuid/refund", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("payment not refundable", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupRefundMux(t)

		paymentID := uuid.New()

		cmd.EXPECT().Execute(mock.Anything, paymentID).
			Return(apperror.ErrBadRequest)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/admin/payments/"+paymentID.String()+"/refund", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})
}

func setupRefundMux(t *testing.T) (*http.ServeMux, *MockRefunder) {
	cmd := NewMockRefunder(t)

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/admin")
	admin.HandleFunc("POST /payments/{id}/refund", New(cmd).Refund)

	return mux, cmd
}
