package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/review"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type publicHandler struct {
	service   *review.Service
	validator *validator.Validator
}

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

type createReviewRequest struct {
	OrderID uuid.UUID `json:"order_id" validate:"required"`
	Rating  int       `json:"rating" validate:"required,min=1,max=5"`
	Title   string    `json:"title" validate:"max=255"`
	Body    string    `json:"body"`
}

func (r createReviewRequest) toCreateParams() review.CreateParams {
	return review.CreateParams{
		OrderID: r.OrderID,
		Rating:  r.Rating,
		Title:   r.Title,
		Body:    r.Body,
	}
}

func (h *publicHandler) Create(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	productID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[createReviewRequest](w, r, h.validator)
	if !ok {
		return
	}

	rv, err := h.service.Create(r.Context(), uc.UserID, productID, req.toCreateParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toReviewResponse(*rv))
}
