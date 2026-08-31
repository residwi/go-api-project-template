package request

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

//nolint:gochecknoglobals // one shared validator: go-playground caches struct metadata per type, so a per-call instance would re-reflect every request
var validate = validator.New()

func Bind[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var req T
	if err := decodeJSON(w, r, &req); err != nil {
		response.HandleErr(w, err)
		return req, false
	}
	if details := validationDetails(req); len(details) > 0 {
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

func validationDetails(s any) map[string]any {
	err := validate.Struct(s)
	if err == nil {
		return nil
	}

	var fieldErrors validator.ValidationErrors
	if !errors.As(err, &fieldErrors) {
		return map[string]any{"error": err.Error()}
	}

	details := make(map[string]any, len(fieldErrors))
	for _, e := range fieldErrors {
		field := strings.ToLower(e.Field()[:1]) + e.Field()[1:]
		details[field] = formatError(e)
	}

	return details
}

func formatError(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "this field is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return fmt.Sprintf("must be at least %s characters", e.Param())
	case "max":
		return fmt.Sprintf("must be at most %s characters", e.Param())
	case "oneof":
		return fmt.Sprintf("must be one of: %s", e.Param())
	case "uuid":
		return "must be a valid UUID"
	case "url":
		return "must be a valid URL"
	case "gte":
		return fmt.Sprintf("must be greater than or equal to %s", e.Param())
	case "lte":
		return fmt.Sprintf("must be less than or equal to %s", e.Param())
	default:
		return fmt.Sprintf("failed on %s validation", e.Tag())
	}
}
