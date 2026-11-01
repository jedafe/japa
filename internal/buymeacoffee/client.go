// Package buymeacoffee — minimal REST client for the Buy Me a Coffee
// personal-access-token API (§125 tips:bmc_sync donation import).
// Only the /supporters endpoint is needed.
package buymeacoffee

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const baseURL = "https://www.buymeacoffee.com/api/v1"

// Supporter is one imported donation.
type Supporter struct {
	ID          string // transaction_id (falls back to support_id)
	AmountCents int64
	Currency    string
	Name        string
	Email       string
}

// Client calls the BMC API.
type Client struct {
	http *http.Client
}

// New returns a Client with sane timeouts.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second}}
}

type supportersResp struct {
	Data []struct {
		SupportID       any    `json:"support_id"`
		TransactionID   any    `json:"transaction_id"`
		SupportCoffees  any    `json:"support_coffees"`
		CoffeePrice     any    `json:"support_coffee_price"`
		SupporterName   string `json:"supporter_name"`
		PayerName       string `json:"payer_name"`
		SupportEmail    string `json:"support_email"`
		PayerEmail      string `json:"payer_email"`
		SupportCurrency string `json:"support_currency"`
		IsRefunded      bool   `json:"is_refunded"`
	} `json:"data"`
	CurrentPage int `json:"current_page"`
	LastPage    int `json:"last_page"`
}

func anyStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	}
	return ""
}

func anyFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

// ListSupporters pages GET /supporters. page is 1-based; returns the
// supporters plus whether more pages remain.
func (c *Client) ListSupporters(ctx context.Context, token string, page int) ([]Supporter, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/supporters?page=%d", baseURL, page), nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, false, fmt.Errorf("buymeacoffee: invalid token (status %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("buymeacoffee: supporters status %d", resp.StatusCode)
	}

	var body supportersResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, false, err
	}

	out := make([]Supporter, 0, len(body.Data))
	for _, s := range body.Data {
		if s.IsRefunded {
			continue
		}
		id := anyStr(s.TransactionID)
		if id == "" {
			id = anyStr(s.SupportID)
		}
		name := s.SupporterName
		if name == "" {
			name = s.PayerName
		}
		email := s.SupportEmail
		if email == "" {
			email = s.PayerEmail
		}
		currency := s.SupportCurrency
		if currency == "" {
			currency = "USD"
		}
		out = append(out, Supporter{
			ID:          id,
			AmountCents: int64(anyFloat(s.SupportCoffees) * anyFloat(s.CoffeePrice) * 100),
			Currency:    currency,
			Name:        name,
			Email:       email,
		})
	}
	return out, body.CurrentPage < body.LastPage, nil
}
