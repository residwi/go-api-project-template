package auth

import (
	"fmt"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

var (
	ErrInvalidCredentials = fmt.Errorf("%w: invalid credentials", errs.ErrUnauthorized)
	ErrInvalidToken       = fmt.Errorf("%w: invalid token", errs.ErrUnauthorized)
	ErrAccountDeactivated = fmt.Errorf("%w: account is deactivated", errs.ErrUnauthorized)
	ErrTokenRevoked       = fmt.Errorf("%w: token has been revoked", errs.ErrUnauthorized)
)
