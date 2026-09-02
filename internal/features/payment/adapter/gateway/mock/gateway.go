package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/residwi/go-api-project-template/internal/features/payment"
)

type Gateway struct {
	httpClient *http.Client
	baseURL    string
}

func New(baseURL string, timeout time.Duration) *Gateway {
	return &Gateway{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    baseURL,
	}
}

func (g *Gateway) Charge(ctx context.Context, req payment.GatewayChargeRequest) (payment.GatewayChargeResponse, error) {
	body, err := json.Marshal(chargeRequestFrom(req))
	if err != nil {
		return payment.GatewayChargeResponse{}, fmt.Errorf("marshaling charge request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/charge", bytes.NewReader(body))
	if err != nil {
		return payment.GatewayChargeResponse{}, fmt.Errorf("creating charge request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return payment.GatewayChargeResponse{}, fmt.Errorf("calling charge endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return payment.GatewayChargeResponse{}, fmt.Errorf("charge endpoint returned status %d", resp.StatusCode)
	}

	var result ChargeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return payment.GatewayChargeResponse{}, fmt.Errorf("decoding charge response: %w", err)
	}

	return result.toPayment(), nil
}

func (g *Gateway) Refund(ctx context.Context, req payment.GatewayRefundRequest) (payment.GatewayRefundResponse, error) {
	body, err := json.Marshal(refundRequestFrom(req))
	if err != nil {
		return payment.GatewayRefundResponse{}, fmt.Errorf("marshaling refund request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/refund", bytes.NewReader(body))
	if err != nil {
		return payment.GatewayRefundResponse{}, fmt.Errorf("creating refund request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return payment.GatewayRefundResponse{}, fmt.Errorf("calling refund endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return payment.GatewayRefundResponse{}, fmt.Errorf("refund endpoint returned status %d", resp.StatusCode)
	}

	var result RefundResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return payment.GatewayRefundResponse{}, fmt.Errorf("decoding refund response: %w", err)
	}

	return result.toPayment(), nil
}
