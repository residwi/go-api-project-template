package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/review/create"
	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// ReviewCreator is what Handler needs from create.Command: create.Command
// satisfies it directly, so nothing sits between them, and the
// mockery-generated mock is the other implementation, used in handler_test.go.
type ReviewCreator interface {
	Execute(ctx context.Context, userID, productID uuid.UUID, p create.Params) (*domain.Review, error)
}

type Handler struct {
	cmd       ReviewCreator
	validator *validator.Validator
}

func New(cmd ReviewCreator, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("POST /products/{id}/reviews", h.create)
}

// Declared here, not shared with review's other slices. Each endpoint holds
// its own copy so one endpoint's new field cannot appear in another's
// response. UserID is omitted because a public response naming the reviewer
// would let a scraper correlate purchases to accounts. OrderID exists only so
// Execute can verify provenance. Status would be the constant 'published' on
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

func toReviewResponse(rv *domain.Review) reviewResponse {
	return reviewResponse{
		ID:        rv.ID,
		ProductID: rv.ProductID,
		Rating:    rv.Rating,
		Title:     rv.Title,
		Body:      rv.Body,
		CreatedAt: rv.CreatedAt,
	}
}

type createReviewRequest struct {
	OrderID uuid.UUID `json:"order_id" validate:"required"`
	Rating  int       `json:"rating"   validate:"required,min=1,max=5"`
	Title   string    `json:"title"    validate:"max=255"`
	Body    string    `json:"body"`
}

func (r createReviewRequest) toParams() create.Params {
	return create.Params{
		OrderID: r.OrderID,
		Rating:  r.Rating,
		Title:   r.Title,
		Body:    r.Body,
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
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

	rv, err := h.cmd.Execute(r.Context(), uc.UserID, productID, req.toParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toReviewResponse(rv))
}
