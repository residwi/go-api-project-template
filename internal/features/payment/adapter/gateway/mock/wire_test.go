package mock

import "github.com/residwi/go-api-project-template/internal/features/payment"

func wireOf(r payment.GatewayChargeResponse) ChargeResponse {
	return ChargeResponse{
		TransactionID: r.TransactionID,
		Status:        r.Status,
		PaymentURL:    r.PaymentURL,
	}
}

func refundWireOf(r payment.GatewayRefundResponse) RefundResponse {
	return RefundResponse{RefundID: r.RefundID, Status: r.Status}
}
