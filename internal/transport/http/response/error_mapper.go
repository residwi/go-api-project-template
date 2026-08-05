package response

import (
	"errors"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/apperror"
)

func HandleErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperror.ErrNotFound):
		NotFound(w, err.Error())
	case errors.Is(err, apperror.ErrConflict):
		Conflict(w, err.Error())
	case errors.Is(err, apperror.ErrBadRequest):
		BadRequest(w, err.Error())
	case errors.Is(err, apperror.ErrUnauthorized), errors.Is(err, apperror.ErrInvalidCredentials):
		Unauthorized(w, err.Error())
	case errors.Is(err, apperror.ErrForbidden):
		Forbidden(w, err.Error())
	case errors.Is(err, apperror.ErrTokenExpired), errors.Is(err, apperror.ErrInvalidToken):
		Unauthorized(w, err.Error())
	case errors.Is(err, apperror.ErrInsufficientStock):
		Conflict(w, err.Error())
	case errors.Is(err, apperror.ErrCartEmpty):
		BadRequest(w, err.Error())
	case errors.Is(err, apperror.ErrOrderNotPayable):
		BadRequest(w, err.Error())
	case errors.Is(err, apperror.ErrOrderCharging):
		Conflict(w, err.Error())
	case errors.Is(err, apperror.ErrAmountMismatch):
		Conflict(w, err.Error())
	case errors.Is(err, apperror.ErrCouponExhausted):
		Conflict(w, err.Error())
	case errors.Is(err, apperror.ErrFulfillmentFailed):
		Conflict(w, err.Error())
	default:
		InternalError(w)
	}
}
