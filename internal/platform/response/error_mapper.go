package response

import (
	"errors"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/apperror"
)

type AppError struct {
	Status  int
	Message string
	Details map[string]any
	Err     error
}

func NewAppError(status int, message string, err error) *AppError {
	return &AppError{Status: status, Message: message, Err: err}
}

func NewAppErrorWithDetails(status int, message string, details map[string]any, err error) *AppError {
	return &AppError{Status: status, Message: message, Details: details, Err: err}
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func HandleErr(w http.ResponseWriter, err error) {
	if appErr, ok := errors.AsType[*AppError](err); ok {
		Err(w, appErr.Status, appErr.Message, appErr.Details)
		return
	}

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
	case errors.Is(err, apperror.ErrInsufficientFunds):
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
