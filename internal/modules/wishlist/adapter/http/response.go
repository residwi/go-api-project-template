package http

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/wishlist/domain"
)

type itemResponse struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	CreatedAt time.Time `json:"created_at"`
}

func toItemResponse(it domain.Item) itemResponse {
	return itemResponse{
		ID:        it.ID,
		ProductID: it.ProductID,
		CreatedAt: it.CreatedAt,
	}
}
