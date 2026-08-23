package response

import (
	"errors"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/apperror"
)

func HandleErr(w http.ResponseWriter, err error) {
	statusFor := []struct {
		sentinel error
		status   int
	}{
		{apperror.ErrNotFound, http.StatusNotFound},
		{apperror.ErrConflict, http.StatusConflict},
		{apperror.ErrBadRequest, http.StatusBadRequest},
		{apperror.ErrUnauthorized, http.StatusUnauthorized},
		{apperror.ErrInvalidCredentials, http.StatusUnauthorized},
		{apperror.ErrForbidden, http.StatusForbidden},
		{apperror.ErrTokenExpired, http.StatusUnauthorized},
		{apperror.ErrInvalidToken, http.StatusUnauthorized},
		{apperror.ErrInsufficientStock, http.StatusConflict},
		{apperror.ErrCartEmpty, http.StatusBadRequest},
		{apperror.ErrOrderNotPayable, http.StatusBadRequest},
		{apperror.ErrOrderCharging, http.StatusConflict},
		{apperror.ErrAmountMismatch, http.StatusConflict},
		{apperror.ErrCouponExhausted, http.StatusConflict},
		{apperror.ErrFulfillmentFailed, http.StatusConflict},
	}

	for _, m := range statusFor {
		if errors.Is(err, m.sentinel) {
			Err(w, m.status, err.Error(), nil)
			return
		}
	}

	InternalError(w)
}
