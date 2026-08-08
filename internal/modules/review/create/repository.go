package create

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
)

// Repository is create's own storage. Its only implementation is
// create/postgres, constructed in review/module.go.
type Repository interface {
	Create(ctx context.Context, rv *domain.Review) error
	HasUserReviewed(ctx context.Context, userID, productID uuid.UUID) (bool, error)
}
