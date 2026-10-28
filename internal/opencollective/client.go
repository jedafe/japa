// Package opencollective implements the §125 Open Collective GraphQL v2
// client — token verification and CREDIT-transaction (contribution)
// enumeration for collective sync.
//
// Ref: japaconnect-engineering/sections/125-donations-tips.md (Open Collective)
package opencollective

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL is the Open Collective GraphQL v2 endpoint.
const DefaultBaseURL = "https://api.opencollective.com/graphql/v2"

// ErrUnauthorized is returned when OC rejects the supplied token.
var ErrUnauthorized = errors.New("opencollective: unauthorized")

// Contribution is a normalized incoming OC contribution.
type Contribution struct {
	ID          string
	AmountCents int64
	Currency    string
	FromName    string
	FromSlug    string
	FromEmail   string
	CreatedAt   time.Time
}

// Client talks to the OC GraphQL API.
type Client struct {
	http    *http.Client
	baseURL string
}

// New creates a client against the production endpoint.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}, baseURL: DefaultBaseURL}
}

// NewWithBaseURL creates a client against a custom endpoint (tests).
func NewWithBaseURL(baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{http: hc, baseURL: baseURL}
}

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphQLError  `json:"errors"`
}

func (c *Client) do(ctx context.Context, token, query string, variables map[string]any, out *json.RawMessage) error {
	body, err := json.Marshal(graphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		// OC personal tokens authenticate via the Personal-Token header.
		req.Header.Set("Personal-Token", token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("opencollective: status %d", resp.StatusCode)
	}

	var envelope graphQLResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("opencollective: decoding response: %w", err)
	}
	if len(envelope.Errors) > 0 {
		return errors.New(envelope.Errors[0].Message)
	}
	if out != nil {
		*out = envelope.Data
	}
	return nil
}

// VerifyToken validates a personal token and returns the account slug it
// belongs to. OC's loggedInAccount root field resolves the token holder.
func (c *Client) VerifyToken(ctx context.Context, token string) (string, error) {
	const q = `query { loggedInAccount { slug name } }`
	var data json.RawMessage
	if err := c.do(ctx, token, q, nil, &data); err != nil {
		return "", err
	}
	var out struct {
		LoggedInAccount *struct {
			Slug string `json:"slug"`
		} `json:"loggedInAccount"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("opencollective: decoding viewer: %w", err)
	}
	if out.LoggedInAccount == nil || out.LoggedInAccount.Slug == "" {
		return "", ErrUnauthorized
	}
	return out.LoggedInAccount.Slug, nil
}

const contributionsQuery = `query($slug: String!, $limit: Int!, $offset: Int!) {
  account(slug: $slug) {
    transactions(type: CREDIT, limit: $limit, offset: $offset,
                 orderBy: {field: CREATED_AT, direction: DESC}) {
      totalCount
      nodes {
        id type kind createdAt
        amount { valueInCents currency }
        fromAccount { name slug }
      }
    }
  }
}`

type transactionsResponse struct {
	Account *struct {
		Transactions struct {
			TotalCount int `json:"totalCount"`
			Nodes      []struct {
				ID          string    `json:"id"`
				Type        string    `json:"type"`
				Kind        string    `json:"kind"`
				CreatedAt   time.Time `json:"createdAt"`
				Amount      struct {
					ValueInCents int64  `json:"valueInCents"`
					Currency     string `json:"currency"`
				} `json:"amount"`
				FromAccount *struct {
					Name string `json:"name"`
					Slug string `json:"slug"`
				} `json:"fromAccount"`
			} `json:"nodes"`
		} `json:"transactions"`
	} `json:"account"`
}

// ListContributions returns incoming CONTRIBUTION transactions for a
// collective. Pagination follows the OC limit/offset contract; the caller
// stops when len(page) < limit.
func (c *Client) ListContributions(ctx context.Context, slug, token string, limit, offset int) ([]Contribution, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var data json.RawMessage
	err := c.do(ctx, token, contributionsQuery, map[string]any{
		"slug": slug, "limit": limit, "offset": offset,
	}, &data)
	if err != nil {
		return nil, err
	}
	var out transactionsResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("opencollective: decoding transactions: %w", err)
	}
	if out.Account == nil {
		return nil, ErrUnauthorized
	}
	var contribs []Contribution
	for _, n := range out.Account.Transactions.Nodes {
		if n.Kind != "CONTRIBUTION" {
			continue // skip host-fee shares and other credit kinds
		}
		c := Contribution{
			ID:          n.ID,
			AmountCents: n.Amount.ValueInCents,
			Currency:    n.Amount.Currency,
			CreatedAt:   n.CreatedAt,
		}
		if n.FromAccount != nil {
			c.FromName = n.FromAccount.Name
			c.FromSlug = n.FromAccount.Slug
		}
		contribs = append(contribs, c)
	}
	return contribs, nil
}
