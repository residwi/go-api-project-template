package response

import (
	"errors"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core"
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
	var appErr *AppError
	if errors.As(err, &appErr) {
		Err(w, appErr.Status, appErr.Message, appErr.Details)
		return
	}

	switch {
	case errors.Is(err, core.ErrNotFound):
		NotFound(w, err.Error())
	case errors.Is(err, core.ErrConflict):
		Conflict(w, err.Error())
	case errors.Is(err, core.ErrBadRequest):
		BadRequest(w, err.Error())
	case errors.Is(err, core.ErrUnauthorized), errors.Is(err, core.ErrInvalidCredentials):
		Unauthorized(w, err.Error())
	case errors.Is(err, core.ErrForbidden):
		Forbidden(w, err.Error())
	case errors.Is(err, core.ErrTokenExpired), errors.Is(err, core.ErrInvalidToken):
		Unauthorized(w, err.Error())
	case errors.Is(err, core.ErrInsufficientStock):
		Conflict(w, err.Error())
	case errors.Is(err, core.ErrCartEmpty):
		BadRequest(w, err.Error())
	case errors.Is(err, core.ErrOrderNotPayable):
		BadRequest(w, err.Error())
	case errors.Is(err, core.ErrOrderCharging):
		Conflict(w, err.Error())
	case errors.Is(err, core.ErrAmountMismatch):
		Conflict(w, err.Error())
	case errors.Is(err, core.ErrCouponExhausted):
		Conflict(w, err.Error())
	case errors.Is(err, core.ErrFulfillmentFailed):
		Conflict(w, err.Error())
	default:
		InternalError(w)
	}
}
