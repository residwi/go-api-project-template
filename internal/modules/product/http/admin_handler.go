package http

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type adminHandler struct {
	service   *product.Service
	validator *validator.Validator
}

// Keeps SKU and Status, which the public productResponse drops: an operator
// reconciles inventory by SKU and needs draft and archived to look different.
type adminProductResponse struct {
	ID             uuid.UUID       `json:"id"`
	CategoryID     *uuid.UUID      `json:"category_id,omitempty"`
	Name           string          `json:"name"`
	Slug           string          `json:"slug"`
	Description    *string         `json:"description,omitempty"`
	Price          int64           `json:"price"`
	CompareAtPrice *int64          `json:"compare_at_price,omitempty"`
	Currency       string          `json:"currency"`
	SKU            *string         `json:"sku,omitempty"`
	Status         string          `json:"status"`
	StockQuantity  int             `json:"stock_quantity"`
	Images         []imageResponse `json:"images,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func toAdminProductResponse(p *product.Product) adminProductResponse {
	return adminProductResponse{
		ID:             p.ID,
		CategoryID:     p.CategoryID,
		Name:           p.Name,
		Slug:           p.Slug,
		Description:    p.Description,
		Price:          p.Price.Amount,
		CompareAtPrice: compareAtPriceAmount(p.CompareAtPrice),
		Currency:       p.Price.Currency,
		SKU:            p.SKU,
		Status:         p.Status,
		StockQuantity:  p.Availability.OnHand,
		Images:         toImageResponses(p.Images),
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

type createProductRequest struct {
	CategoryID     *uuid.UUID `json:"category_id"      validate:"omitempty"`
	Name           string     `json:"name"             validate:"required,min=1,max=255"`
	Description    *string    `json:"description"      validate:"omitempty"`
	Price          int64      `json:"price"            validate:"required,min=0"`
	CompareAtPrice *int64     `json:"compare_at_price" validate:"omitempty,min=0"`
	Currency       string     `json:"currency"         validate:"omitempty,len=3"`
	SKU            *string    `json:"sku"              validate:"omitempty,max=100"`
	Status         string     `json:"status"           validate:"omitempty,oneof=draft published archived"`
}

// An empty `currency` is passed through: the default is a business rule
// Service.Create owns.
func (r createProductRequest) toCreateParams() product.CreateParams {
	p := product.CreateParams{
		CategoryID:  r.CategoryID,
		Name:        r.Name,
		Description: r.Description,
		Price:       money.New(r.Price, r.Currency),
		SKU:         r.SKU,
		Status:      r.Status,
	}
	if r.CompareAtPrice != nil {
		compareAt := money.New(*r.CompareAtPrice, r.Currency)
		p.CompareAtPrice = &compareAt
	}
	return p
}

func (h *adminHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[createProductRequest](w, r, h.validator)
	if !ok {
		return
	}

	p, err := h.service.Create(r.Context(), req.toCreateParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toAdminProductResponse(p))
}

func (h *adminHandler) List(w http.ResponseWriter, r *http.Request) {
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

func (h *adminHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
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

type updateProductRequest struct {
	CategoryID     *uuid.UUID `json:"category_id"      validate:"omitempty"`
	Name           *string    `json:"name"             validate:"omitempty,min=1,max=255"`
	Description    *string    `json:"description"      validate:"omitempty"`
	Price          *int64     `json:"price"            validate:"omitempty,min=0"`
	CompareAtPrice *int64     `json:"compare_at_price" validate:"omitempty,min=0"`
	Currency       *string    `json:"currency"         validate:"omitempty,len=3"`
	SKU            *string    `json:"sku"              validate:"omitempty,max=100"`
	Status         *string    `json:"status"           validate:"omitempty,oneof=draft published archived"`
}

// The three monetary keys move as one group -- `price` with `currency`,
// optionally `compare_at_price` -- or none at all. Anything in between is a 400
// rather than a guess: any partial combination re-prices or re-labels the row
// in a denomination the client never named, and products stores one currency
// for the whole row.
//
// The validate tags cannot express this: `omitempty` makes each field
// independently optional, and a `required_with` group would surface as the 422
// response.Bind returns, not the 400 a well-formed contradiction deserves.
func (r updateProductRequest) toUpdateParams() (product.UpdateParams, error) {
	p := product.UpdateParams{
		CategoryID:  r.CategoryID,
		Name:        r.Name,
		Description: r.Description,
		SKU:         r.SKU,
		Status:      r.Status,
	}

	switch {
	case r.Price == nil && r.Currency == nil && r.CompareAtPrice == nil:
		return p, nil
	case r.Price == nil || r.Currency == nil:
		return product.UpdateParams{}, fmt.Errorf(
			"%w: price and currency must be supplied together, and compare_at_price requires both",
			apperror.ErrBadRequest)
	}

	price := money.New(*r.Price, *r.Currency)
	p.Price = &price
	if r.CompareAtPrice != nil {
		compareAt := money.New(*r.CompareAtPrice, *r.Currency)
		p.CompareAtPrice = &compareAt
	}
	return p, nil
}

func (h *adminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateProductRequest](w, r, h.validator)
	if !ok {
		return
	}

	params, err := req.toUpdateParams()
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	p, err := h.service.Update(r.Context(), id, params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminProductResponse(p))
}

func (h *adminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
