package response

import (
	"errors"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/apperror"
)

// HandleErr writes the response matching err's sentinel, or a 500 when none
// matches. The table is ordered and first match wins, so two wrapped sentinels
// always resolve the same way.
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
