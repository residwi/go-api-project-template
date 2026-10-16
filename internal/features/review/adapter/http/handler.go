package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
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
	service ReviewManager
}

func NewHandler(service ReviewManager) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	productID, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	req, err := request.Bind[createReviewRequest](w, r)
	if err != nil {
		return err
	}

	rv, err := h.service.Create(r.Context(), uc.UserID, productID, req.OrderID, req.Rating, req.Title, req.Body)
	if err != nil {
		return err
	}

	response.Created(w, toReviewResponse(*rv))
	return nil
}

func (h *Handler) ListByProduct(w http.ResponseWriter, r *http.Request) error {
	productID, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	cursor := paging.ParseCursorPage(r)

	reviews, err := h.service.ListByProduct(r.Context(), productID, cursor)
	if err != nil {
		return err
	}

	out := make([]reviewResponse, len(reviews))
	for i, rv := range reviews {
		out[i] = toReviewResponse(rv)
	}

	response.CursorPage(w, out, cursor.Limit, func(rv reviewResponse) (time.Time, uuid.UUID) {
		return rv.CreatedAt, rv.ID
	})
	return nil
}
