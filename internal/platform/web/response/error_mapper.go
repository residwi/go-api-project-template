package response

import (
	"errors"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

type ValidationError struct {
	Details map[string]any
}

func (e *ValidationError) Error() string {
	return "validation failed"
}

func (e *ValidationError) Unwrap() error {
	return errs.ErrBadRequest
}

func HandleErr(w http.ResponseWriter, err error) {
	if ve, ok := errors.AsType[*ValidationError](err); ok {
		Err(w, http.StatusUnprocessableEntity, ve.Error(), ve.Details)
		return
	}

	kind := errs.Kind(err)
	if kind == nil {
		InternalError(w)
		return
	}

	statusFor := map[error]int{
		errs.ErrNotFound:     http.StatusNotFound,
		errs.ErrConflict:     http.StatusConflict,
		errs.ErrBadRequest:   http.StatusBadRequest,
		errs.ErrUnauthorized: http.StatusUnauthorized,
		errs.ErrForbidden:    http.StatusForbidden,
	}

	Err(w, statusFor[kind], err.Error(), nil)
}
