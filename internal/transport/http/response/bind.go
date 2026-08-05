package response

import "net/http"

// Validator is declared here so this package need not import the validator
// package; *validator.Validator satisfies it.
type Validator interface {
	Validate(s any) map[string]any
}

// Bind writes the error response itself -- 400 for a malformed body, 422 for
// invalid fields -- and returns ok=false, so the caller can simply return.
func Bind[T any](w http.ResponseWriter, r *http.Request, v Validator) (T, bool) {
	var req T
	if err := DecodeJSON(w, r, &req); err != nil {
		HandleErr(w, err)
		return req, false
	}
	if errs := v.Validate(req); len(errs) > 0 {
		ValidationErr(w, errs)
		return req, false
	}
	return req, true
}
