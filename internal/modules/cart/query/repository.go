package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/cart/domain"
)

// Repository is query's own storage. Its only implementation is
// query/postgres, constructed in cart/module.go.
type Repository interface {
	GetCart(ctx context.Context, userID uuid.UUID) (*domain.Cart, error)
}
