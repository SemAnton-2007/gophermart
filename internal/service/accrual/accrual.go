package accrual

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"gophermart/internal/model"
)

type AccrualClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewAccrualClient(baseURL string) *AccrualClient {
	return &AccrualClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type AccrualResponse struct {
	Order   string            `json:"order"`
	Status  model.OrderStatus `json:"status"`
	Accrual float64           `json:"accrual,omitempty"`
}

func (c *AccrualClient) GetAccrual(ctx context.Context, orderNumber string) (*AccrualResponse, error) {
	url := fmt.Sprintf("%s/api/orders/%s", c.baseURL, orderNumber)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var accrualResp AccrualResponse
		if err := json.NewDecoder(resp.Body).Decode(&accrualResp); err != nil {
			return nil, err
		}
		return &accrualResp, nil
	case http.StatusNoContent:
		return nil, nil
	case http.StatusTooManyRequests:
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter == "" {
			retryAfter = "60"
		}
		return nil, fmt.Errorf("rate limit exceeded, retry after %s seconds", retryAfter)
	default:
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}
