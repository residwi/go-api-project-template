package errs

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrBadRequest   = errors.New("bad request")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
)

func Kind(err error) error {
	for _, kind := range []error{ErrNotFound, ErrConflict, ErrBadRequest, ErrUnauthorized, ErrForbidden} {
		if errors.Is(err, kind) {
			return kind
		}
	}

	return nil
}
