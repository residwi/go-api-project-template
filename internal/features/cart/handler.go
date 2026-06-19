package cart

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type handler struct {
	service   *Service
	validator *validator.Validator
}

func (h *handler) GetCart(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	cart, err := h.service.GetCart(r.Context(), uc.UserID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, cart)
}

func (h *handler) AddItem(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[AddItemRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.AddItem(r.Context(), uc.UserID, req); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, nil)
}

func (h *handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	productID, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	req, ok := response.Bind[UpdateItemRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.UpdateQuantity(r.Context(), uc.UserID, productID, req); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}

func (h *handler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	productID, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	if err := h.service.RemoveItem(r.Context(), uc.UserID, productID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}

func (h *handler) Clear(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	if err := h.service.Clear(r.Context(), uc.UserID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
