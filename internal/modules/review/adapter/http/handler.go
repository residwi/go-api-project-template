package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
	"github.com/residwi/go-api-project-template/internal/server/middleware"
)

type ReviewManager interface {
	Create(
		ctx context.Context,
		userID, productID, orderID uuid.UUID,
		rating int,
		title, body string,
	) (*domain.Review, error)
	ListByProduct(ctx context.Context, productID uuid.UUID, cursor paging.CursorPage) ([]domain.Review, error)
}

type Handler struct {
	service   ReviewManager
	validator *validator.Validator
}

func NewHandler(service ReviewManager, v *validator.Validator) *Handler {
	return &Handler{service: service, validator: v}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
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

	rv, err := h.service.Create(r.Context(), uc.UserID, productID, req.OrderID, req.Rating, req.Title, req.Body)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toReviewResponse(*rv))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
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
