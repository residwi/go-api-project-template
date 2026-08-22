package category

import (
	"context"

	"github.com/google/uuid"
)

// ProductCounter is satisfied by product's query use case. It lives here,
// not on Delete alone, because module.go -- now service.go -- is where a
// port fed by only one method still gets declared once the slice that used
// to own it is gone.
type ProductCounter interface {
	CountPublished(ctx context.Context, categoryID uuid.UUID) (int, error)
}
