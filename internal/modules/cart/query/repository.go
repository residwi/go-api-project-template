package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/cart/domain"
)

type Repository interface {
	GetCart(ctx context.Context, userID uuid.UUID) (*domain.Cart, error)
}
