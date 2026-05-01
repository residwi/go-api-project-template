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
	uc, ok := middleware.GetUserContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
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
	uc, ok := middleware.GetUserContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}

	var req AddItemRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		response.HandleErr(w, err)
		return
	}

	if errors := h.validator.Validate(req); errors != nil {
		response.ValidationErr(w, errors)
		return
	}

	if err := h.service.AddItem(r.Context(), uc.UserID, req); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, nil)
}

func (h *handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.GetUserContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}

	productID, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	var req UpdateItemRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		response.HandleErr(w, err)
		return
	}

	if errors := h.validator.Validate(req); errors != nil {
		response.ValidationErr(w, errors)
		return
	}

	if err := h.service.UpdateQuantity(r.Context(), uc.UserID, productID, req); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}

func (h *handler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.GetUserContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
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
	uc, ok := middleware.GetUserContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}

	if err := h.service.Clear(r.Context(), uc.UserID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
