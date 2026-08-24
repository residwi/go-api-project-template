package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway"
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

func (g *Gateway) Charge(ctx context.Context, req gateway.ChargeRequest) (gateway.ChargeResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return gateway.ChargeResponse{}, fmt.Errorf("marshaling charge request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/charge", bytes.NewReader(body))
	if err != nil {
		return gateway.ChargeResponse{}, fmt.Errorf("creating charge request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return gateway.ChargeResponse{}, fmt.Errorf("calling charge endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return gateway.ChargeResponse{}, fmt.Errorf("charge endpoint returned status %d", resp.StatusCode)
	}

	var result gateway.ChargeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return gateway.ChargeResponse{}, fmt.Errorf("decoding charge response: %w", err)
	}

	return result, nil
}

func (g *Gateway) Refund(ctx context.Context, req gateway.RefundRequest) (gateway.RefundResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return gateway.RefundResponse{}, fmt.Errorf("marshaling refund request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/refund", bytes.NewReader(body))
	if err != nil {
		return gateway.RefundResponse{}, fmt.Errorf("creating refund request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return gateway.RefundResponse{}, fmt.Errorf("calling refund endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return gateway.RefundResponse{}, fmt.Errorf("refund endpoint returned status %d", resp.StatusCode)
	}

	var result gateway.RefundResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return gateway.RefundResponse{}, fmt.Errorf("decoding refund response: %w", err)
	}

	return result, nil
}
