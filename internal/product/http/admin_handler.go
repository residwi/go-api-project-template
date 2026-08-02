package http

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/product"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type adminHandler struct {
	service   *product.Service
	validator *validator.Validator
}

// adminProductResponse is the admin wire contract -- unlike the public
// productResponse (public_handler.go), it keeps SKU and Status: an operator
// needs a SKU to reconcile inventory and needs to see draft/archived
// products distinctly from published ones. Used by every admin endpoint
// that returns a product body: this file's Create, List, Get, and Update.
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
	CategoryID     *uuid.UUID `json:"category_id" validate:"omitempty"`
	Name           string     `json:"name" validate:"required,min=1,max=255"`
	Description    *string    `json:"description" validate:"omitempty"`
	Price          int64      `json:"price" validate:"required,min=0"`
	CompareAtPrice *int64     `json:"compare_at_price" validate:"omitempty,min=0"`
	Currency       string     `json:"currency" validate:"omitempty,len=3"`
	SKU            *string    `json:"sku" validate:"omitempty,max=100"`
	Status         string     `json:"status" validate:"omitempty,oneof=draft published archived"`
}

// toCreateParams pairs the request's two amounts with its single `currency`
// key, so both arrive at the service denominated and agreeing with each other.
//
// `currency` is optional on the wire and stays so: an empty currency reaches
// product.Service.Create, which denominates the product in its default. The
// default is a business rule, not a transport one, so it is not applied here.
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
		Page:     page.Page,
		PageSize: page.PageSize,
		Status:   r.URL.Query().Get("status"),
		Search:   r.URL.Query().Get("search"),
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

	response.Paginated(w, paging.NewOffsetPageResult(out, page, total))
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
	CategoryID     *uuid.UUID `json:"category_id" validate:"omitempty"`
	Name           *string    `json:"name" validate:"omitempty,min=1,max=255"`
	Description    *string    `json:"description" validate:"omitempty"`
	Price          *int64     `json:"price" validate:"omitempty,min=0"`
	CompareAtPrice *int64     `json:"compare_at_price" validate:"omitempty,min=0"`
	Currency       *string    `json:"currency" validate:"omitempty,len=3"`
	SKU            *string    `json:"sku" validate:"omitempty,max=100"`
	Status         *string    `json:"status" validate:"omitempty,oneof=draft published archived"`
}

// toUpdateParams maps the request onto product.UpdateParams, whose amounts are
// now money.Money and therefore inseparable from their currency.
//
// The three monetary keys move as one group: supply `price` and `currency`
// together (optionally with `compare_at_price`), or none of the three. Anything
// in between is rejected here as a 400 rather than completed with a guess:
//
//   - `price` without `currency` used to inherit the stored currency, which
//     means a client could re-price a product in a denomination it never named
//     and get a 200 back. Silently inheriting hides that bug; refusing surfaces
//     it at the one moment someone is looking.
//   - `currency` without `price` is the same mistake read backwards -- a
//     re-denomination that leaves the old amount standing. products stores one
//     currency for the whole row, so this quietly re-labels compare_at_price too.
//   - `compare_at_price` without `price` would be written under the price's
//     currency rather than the one it was sent with, for the same reason.
//
// The validate tags cannot express this: `omitempty` makes each field
// independently optional, and a `required_with` group would surface as a 422
// from response.Bind, not the 400 a contradictory-but-well-formed body deserves.
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
		// No monetary key at all: the product's price is left exactly as stored.
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
