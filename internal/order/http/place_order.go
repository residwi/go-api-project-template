package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/order"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// addressRequest maps to order.Address explicitly rather than binding the
// domain struct directly, mirroring addressResponse's rationale.
type addressRequest struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zip_code"`
	Country string `json:"country"`
}

func (r *addressRequest) toAddress() *order.Address {
	if r == nil {
		return nil
	}
	return &order.Address{
		Street:  r.Street,
		City:    r.City,
		State:   r.State,
		ZipCode: r.ZipCode,
		Country: r.Country,
	}
}

type placeOrderRequest struct {
	PaymentMethodID string          `json:"payment_method_id" validate:"required"`
	CouponCode      *string         `json:"coupon_code,omitempty"`
	ShippingAddress *addressRequest `json:"shipping_address,omitempty"`
	BillingAddress  *addressRequest `json:"billing_address,omitempty"`
	Notes           string          `json:"notes,omitempty"`
}

func (r placeOrderRequest) toPlaceParams() order.PlaceParams {
	return order.PlaceParams{
		PaymentMethodID: r.PaymentMethodID,
		CouponCode:      r.CouponCode,
		ShippingAddress: r.ShippingAddress.toAddress(),
		BillingAddress:  r.BillingAddress.toAddress(),
		Notes:           r.Notes,
	}
}

// placeOrderResponse keeps the pre-refactor "order" envelope: integration
// tests (internal/transport/http/router_test.go) decode
// data.order.{id,coupon_code,subtotal_amount,discount_amount,total_amount},
// so flattening this to just orderResponse would be a real behavior change,
// not just a shape change.
type placeOrderResponse struct {
	Order orderResponse `json:"order"`
}

func toPlaceOrderResponse(r *order.PlaceResult) placeOrderResponse {
	return placeOrderResponse{Order: toOrderResponse(r.Order)}
}

func (h *publicHandler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		response.BadRequest(w, "Idempotency-Key header is required")
		return
	}

	req, ok := response.Bind[placeOrderRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.PlaceOrder(r.Context(), uc.UserID, req.toPlaceParams(), idempotencyKey)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toPlaceOrderResponse(result))
}
