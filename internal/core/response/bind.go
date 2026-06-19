package response

import "net/http"

// Validator is the validation surface Bind needs. *validator.Validator
// satisfies it, so handlers pass their existing validator without the response
// package having to depend on the validator package.
type Validator interface {
	Validate(s any) map[string]any
}

// Bind decodes the JSON request body into a T and validates it. On failure it
// writes the matching error response — 400 for a malformed body, 422 for
// invalid fields — and returns ok=false, so the caller can simply return. On
// success it returns the populated value and ok=true.
func Bind[T any](w http.ResponseWriter, r *http.Request, v Validator) (T, bool) {
	var req T
	if err := DecodeJSON(w, r, &req); err != nil {
		HandleErr(w, err)
		return req, false
	}
	if errs := v.Validate(req); errs != nil {
		ValidationErr(w, errs)
		return req, false
	}
	return req, true
}
