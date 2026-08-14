package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/category/create"
	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type CategoryCreator interface {
	Execute(ctx context.Context, p create.Params) (*domain.Category, error)
}

type Handler struct {
	cmd       CategoryCreator
	validator *validator.Validator
}

func New(cmd CategoryCreator, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

func (h *Handler) RegisterHTTP(admin *middleware.RouteGroup) {
	admin.HandleFunc("POST /categories", h.Create)
}

type categoryResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Description *string    `json:"description,omitempty"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
	SortOrder   int        `json:"sort_order"`
	Active      bool       `json:"active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func toCategoryResponse(c *domain.Category) categoryResponse {
	return categoryResponse{
		ID:          c.ID,
		Name:        c.Name,
		Slug:        c.Slug,
		Description: c.Description,
		ParentID:    c.ParentID,
		SortOrder:   c.SortOrder,
		Active:      c.Active,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

type createCategoryRequest struct {
	Name        string     `json:"name"        validate:"required,min=1,max=255"`
	Description *string    `json:"description" validate:"omitempty"`
	ParentID    *uuid.UUID `json:"parent_id"   validate:"omitempty"`
	SortOrder   *int       `json:"sort_order"  validate:"omitempty,min=0"`
	Active      *bool      `json:"active"`
}

func (r createCategoryRequest) toParams() create.Params {
	return create.Params{
		Name:        r.Name,
		Description: r.Description,
		ParentID:    r.ParentID,
		SortOrder:   r.SortOrder,
		Active:      r.Active,
	}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[createCategoryRequest](w, r, h.validator)
	if !ok {
		return
	}

	cat, err := h.cmd.Execute(r.Context(), req.toParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toCategoryResponse(cat))
}
