package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// ReviewReader is what Handler needs from query.Reader: query.Reader
// satisfies it directly, so nothing sits between them, and the
// mockery-generated mock is the other implementation, used in handler_test.go.
type ReviewReader interface {
	ListByProduct(ctx context.Context, productID uuid.UUID, cursor paging.CursorPage) ([]domain.Review, error)
}

type Handler struct {
	reader ReviewReader
}

func New(reader ReviewReader) *Handler {
	return &Handler{reader: reader}
}

func (h *Handler) RegisterHTTP(api *middleware.RouteGroup) {
	api.HandleFunc("GET /products/{id}/reviews", h.list)
}

// Declared here, not shared with review's other slices. Each endpoint holds
// its own copy so one endpoint's new field cannot appear in another's
// response. UserID is omitted because a public response naming the reviewer
// would let a scraper correlate purchases to accounts. OrderID exists only so
// create can verify provenance. Status would be the constant 'published' on
// every path this returns, and nothing here ever mutates a review after
// creation.
type reviewResponse struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	Rating    int       `json:"rating"`
	Title     string    `json:"title,omitempty"`
	Body      string    `json:"body,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func toReviewResponse(rv domain.Review) reviewResponse {
	return reviewResponse{
		ID:        rv.ID,
		ProductID: rv.ProductID,
		Rating:    rv.Rating,
		Title:     rv.Title,
		Body:      rv.Body,
		CreatedAt: rv.CreatedAt,
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	productID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	cursor := paging.ParseCursorPage(r)

	reviews, err := h.reader.ListByProduct(r.Context(), productID, cursor)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]reviewResponse, len(reviews))
	for i, rv := range reviews {
		out[i] = toReviewResponse(rv)
	}

	response.CursorPage(w, out, cursor.Limit, func(rv reviewResponse) (time.Time, uuid.UUID) {
		return rv.CreatedAt, rv.ID
	})
}
