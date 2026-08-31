package request

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type Validator interface {
	Validate(s any) map[string]any
}

func Bind[T any](w http.ResponseWriter, r *http.Request, v Validator) (T, bool) {
	var req T
	if err := decodeJSON(w, r, &req); err != nil {
		response.HandleErr(w, err)
		return req, false
	}
	if details := v.Validate(req); len(details) > 0 {
		response.ValidationErr(w, details)
		return req, false
	}
	return req, true
}

func ParseUUIDParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, "invalid "+name)
		return uuid.Nil, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return fmt.Errorf("%w: request body too large", errs.ErrBadRequest)
		}
		return fmt.Errorf("%w: %s", errs.ErrBadRequest, err.Error())
	}
	return nil
}
