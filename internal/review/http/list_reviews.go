package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/review"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// reviewResponse deliberately omits UserID: naming the reviewer's id on a
// public response would let a scraper correlate purchases to accounts.
// OrderID is dropped too -- it exists only so Create can verify provenance,
// a client has no use for it back. Status is dropped because every review
// this endpoint (or Create) can ever return is already StatusPublished --
// ListByProduct filters WHERE status = 'published', so the field would be a
// constant, not information. UpdatedAt is dropped along with it: nothing
// in this feature ever mutates a review after creation.
type reviewResponse struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	Rating    int       `json:"rating"`
	Title     string    `json:"title,omitempty"`
	Body      string    `json:"body,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func toReviewResponse(rv review.Review) reviewResponse {
	return reviewResponse{
		ID:        rv.ID,
		ProductID: rv.ProductID,
		Rating:    rv.Rating,
		Title:     rv.Title,
		Body:      rv.Body,
		CreatedAt: rv.CreatedAt,
	}
}

func (h *publicHandler) ListByProduct(w http.ResponseWriter, r *http.Request) {
	productID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	cursor := paging.ParseCursorPage(r)

	reviews, err := h.service.ListByProduct(r.Context(), productID, cursor)
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
