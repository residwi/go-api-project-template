package http

import (
	"time"

	"github.com/google/uuid"

	orderdomain "github.com/residwi/go-api-project-template/internal/features/order/domain"
	"github.com/residwi/go-api-project-template/internal/features/payment"
)

type addressResponse struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zip_code"`
	Country string `json:"country"`
}

func toAddressResponse(a *orderdomain.Address) *addressResponse {
	if a == nil {
		return nil
	}
	return &addressResponse{
		Street:  a.Street,
		City:    a.City,
		State:   a.State,
		ZipCode: a.ZipCode,
		Country: a.Country,
	}
}

type orderItemResponse struct {
	ID          uuid.UUID `json:"id"`
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	Price       int64     `json:"price"`
	Quantity    int       `json:"quantity"`
	Subtotal    int64     `json:"subtotal"`
	CreatedAt   time.Time `json:"created_at"`
}

func toOrderItemResponse(i orderdomain.Item) orderItemResponse {
	return orderItemResponse{
		ID:          i.ID,
		ProductID:   i.ProductID,
		ProductName: i.ProductName,
		Price:       i.Price.Amount,
		Quantity:    i.Quantity,
		Subtotal:    i.Subtotal.Amount,
		CreatedAt:   i.CreatedAt,
	}
}

type orderResponse struct {
	ID              uuid.UUID           `json:"id"`
	UserID          uuid.UUID           `json:"user_id"`
	Status          orderdomain.Status  `json:"status"`
	SubtotalAmount  int64               `json:"subtotal_amount"`
	DiscountAmount  int64               `json:"discount_amount"`
	TotalAmount     int64               `json:"total_amount"`
	CouponCode      *string             `json:"coupon_code,omitempty"`
	Currency        string              `json:"currency"`
	ShippingAddress *addressResponse    `json:"shipping_address,omitempty"`
	BillingAddress  *addressResponse    `json:"billing_address,omitempty"`
	Notes           string              `json:"notes,omitempty"`
	Items           []orderItemResponse `json:"items,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

func toOrderResponse(o *orderdomain.Order) orderResponse {
	items := make([]orderItemResponse, len(o.Items))
	for i, it := range o.Items {
		items[i] = toOrderItemResponse(it)
	}

	return orderResponse{
		ID:              o.ID,
		UserID:          o.UserID,
		Status:          o.Status,
		SubtotalAmount:  o.Subtotal.Amount,
		DiscountAmount:  o.Discount.Amount,
		TotalAmount:     o.Total.Amount,
		CouponCode:      o.CouponCode,
		Currency:        o.Total.Currency,
		ShippingAddress: toAddressResponse(o.ShippingAddress),
		BillingAddress:  toAddressResponse(o.BillingAddress),
		Notes:           o.Notes,
		Items:           items,
		CreatedAt:       o.CreatedAt,
		UpdatedAt:       o.UpdatedAt,
	}
}

type placeOrderResponse struct {
	Order orderResponse `json:"order"`
}

func toPlaceOrderResponse(o *orderdomain.Order) placeOrderResponse {
	return placeOrderResponse{Order: toOrderResponse(o)}
}

type payResultResponse struct {
	PaymentID  uuid.UUID `json:"payment_id"`
	PaymentURL string    `json:"payment_url,omitempty"`
	Charged    bool      `json:"charged"`
}

func toPayResultResponse(r payment.ChargeResult) payResultResponse {
	return payResultResponse{
		PaymentID:  r.PaymentID,
		PaymentURL: r.PaymentURL,
		Charged:    r.Charged,
	}
}
