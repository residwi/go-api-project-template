package response

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

func HandleErr(w http.ResponseWriter, err error) {
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
