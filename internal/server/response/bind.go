package response

import "net/http"

type Validator interface {
	Validate(s any) map[string]any
}

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
