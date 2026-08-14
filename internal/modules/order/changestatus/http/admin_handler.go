package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type Command interface {
	Execute(ctx context.Context, orderID uuid.UUID, toStatus domain.Status) error
}

type AdminHandler struct {
	cmd       Command
	validator *validator.Validator
}

func NewAdmin(cmd Command, v *validator.Validator) *AdminHandler {
	return &AdminHandler{cmd: cmd, validator: v}
}

type updateStatusRequest struct {
	Status string `json:"status" validate:"required"`
}

func (h *AdminHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateStatusRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.cmd.Execute(r.Context(), id, domain.Status(req.Status)); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
