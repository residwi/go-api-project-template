package response

import (
	"errors"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

func HandleErr(w http.ResponseWriter, err error) {
	statusFor := []struct {
		sentinel error
		status   int
	}{
		{errs.ErrNotFound, http.StatusNotFound},
		{errs.ErrConflict, http.StatusConflict},
		{errs.ErrBadRequest, http.StatusBadRequest},
		{errs.ErrUnauthorized, http.StatusUnauthorized},
		{errs.ErrForbidden, http.StatusForbidden},
	}

	for _, m := range statusFor {
		if errors.Is(err, m.sentinel) {
			Err(w, m.status, err.Error(), nil)
			return
		}
	}

	InternalError(w)
}
