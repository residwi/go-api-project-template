package http

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/money"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type ProductManager interface {
	ListAdmin(ctx context.Context, params product.AdminListParams) ([]domain.Product, int, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Product, error)
	Create(
		ctx context.Context,
		categoryID *uuid.UUID,
		name string,
		description *string,
		price money.Money,
		compareAtPrice *money.Money,
		sku *string,
		status string,
	) (*domain.Product, error)
	Update(
		ctx context.Context,
		id uuid.UUID,
		categoryID *uuid.UUID,
		name *string,
		description *string,
		price *money.Money,
		compareAtPrice *money.Money,
		sku *string,
		status *string,
	) (*domain.Product, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type AdminHandler struct {
	service   ProductManager
	validator *validator.Validator
}

func NewAdminHandler(service ProductManager, v *validator.Validator) *AdminHandler {
	return &AdminHandler{service: service, validator: v}
}

func (h *AdminHandler) List(w http.ResponseWriter, r *http.Request) {
	page := paging.ParseOffsetPage(r)

	params := product.AdminListParams{
		OffsetPage: page,
		Status:     r.URL.Query().Get("status"),
		Search:     r.URL.Query().Get("search"),
	}

	if catID := r.URL.Query().Get("category_id"); catID != "" {
		id, err := uuid.Parse(catID)
		if err != nil {
			response.BadRequest(w, "invalid category_id")
			return
		}
		params.CategoryID = &id
	}

	products, total, err := h.service.ListAdmin(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]adminProductResponse, len(products))
	for i, p := range products {
		out[i] = toAdminProductResponse(&p)
	}

	response.OK(w, paging.NewOffsetPageResult(out, page, total))
}

func (h *AdminHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	p, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminProductResponse(p))
}

func (h *AdminHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := request.Bind[createProductRequest](w, r, h.validator)
	if !ok {
		return
	}

	price := money.New(req.Price, req.Currency)
	var compareAtPrice *money.Money
	if req.CompareAtPrice != nil {
		c := money.New(*req.CompareAtPrice, req.Currency)
		compareAtPrice = &c
	}

	p, err := h.service.Create(
		r.Context(), req.CategoryID, req.Name, req.Description, price, compareAtPrice, req.SKU, req.Status,
	)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toAdminProductResponse(p))
}

func (h *AdminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := request.Bind[updateProductRequest](w, r, h.validator)
	if !ok {
		return
	}

	var price, compareAtPrice *money.Money
	switch {
	case req.Price == nil && req.Currency == nil && req.CompareAtPrice == nil:
	case req.Price == nil || req.Currency == nil:
		response.HandleErr(w, fmt.Errorf(
			"%w: price and currency must be supplied together, and compare_at_price requires both",
			errs.ErrBadRequest))
		return
	default:
		pr := money.New(*req.Price, *req.Currency)
		price = &pr
		if req.CompareAtPrice != nil {
			c := money.New(*req.CompareAtPrice, *req.Currency)
			compareAtPrice = &c
		}
	}

	p, err := h.service.Update(
		r.Context(), id, req.CategoryID, req.Name, req.Description, price, compareAtPrice, req.SKU, req.Status,
	)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminProductResponse(p))
}

func (h *AdminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
