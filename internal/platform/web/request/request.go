package request

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

//nolint:gochecknoglobals // one shared validator: go-playground caches struct metadata per type, so a per-call instance would re-reflect every request
var validate = newValidator()

func Bind[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var req T
	if err := decodeJSON(w, r, &req); err != nil {
		return req, err
	}
	details := validationDetails(req)
	if v, ok := any(&req).(validatable); ok {
		details = withRules(details, v.Validate())
	}
	if len(details) > 0 {
		return req, &response.ValidationError{Details: details}
	}
	return req, nil
}

func RequireUser(r *http.Request) (identity.Identity, error) {
	id, ok := identity.FromContext(r.Context())
	if !ok {
		return identity.Identity{}, fmt.Errorf("%w: authentication required", errs.ErrUnauthorized)
	}
	return id, nil
}

func ParseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: invalid %s", errs.ErrBadRequest, name)
	}
	return id, nil
}

type validatable interface {
	Validate() map[string]string
}

func withRules(details map[string]any, rules map[string]string) map[string]any {
	merged := make(map[string]any, len(details)+len(rules))
	for field, msg := range rules {
		merged[field] = msg
	}
	maps.Copy(merged, details)
	return merged
}

func newValidator() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		return name
	})
	return v
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.UnmarshalRead(r.Body, dst, json.RejectUnknownMembers(true)); err != nil {
		return decodeError(err)
	}
	return nil
}

func decodeError(err error) error {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return fmt.Errorf("%w: request body too large", errs.ErrBadRequest)
	}
	if se, ok := errors.AsType[*jsontext.SyntacticError](err); ok && errors.Is(err, jsontext.ErrDuplicateName) {
		return fmt.Errorf("%w: duplicate field %q", errs.ErrBadRequest, se.JSONPointer.LastToken())
	}
	if se, ok := errors.AsType[*json.SemanticError](err); ok && se.JSONPointer != "" {
		if errors.Is(err, json.ErrUnknownName) {
			return fmt.Errorf("%w: unknown field %q", errs.ErrBadRequest, se.JSONPointer.LastToken())
		}
		path := strings.Join(slices.Collect(se.JSONPointer.Tokens()), ".")
		return fmt.Errorf("%w: invalid type for %q", errs.ErrBadRequest, path)
	}
	return fmt.Errorf("%w: malformed JSON", errs.ErrBadRequest)
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
		details[e.Field()] = formatError(e)
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
		if e.Kind() != reflect.String {
			return fmt.Sprintf("must be greater than or equal to %s", e.Param())
		}
		return fmt.Sprintf("must be at least %s characters", e.Param())
	case "max":
		if e.Kind() != reflect.String {
			return fmt.Sprintf("must be less than or equal to %s", e.Param())
		}
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
