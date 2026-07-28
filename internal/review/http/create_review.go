package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/review"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

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
