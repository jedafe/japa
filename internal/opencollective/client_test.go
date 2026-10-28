package opencollective

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func gqlServer(t *testing.T, data any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Personal-Token") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		payload, _ := json.Marshal(map[string]any{"data": data})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
}

func TestVerifyToken(t *testing.T) {
	srv := gqlServer(t, map[string]any{"loggedInAccount": map[string]any{"slug": "me", "name": "Me"}})
	defer srv.Close()

	c := NewWithBaseURL(srv.URL, srv.Client())
	slug, err := c.VerifyToken(context.Background(), "tok")
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if slug != "me" {
		t.Fatalf("slug = %q", slug)
	}
}

func TestListContributionsFiltersKinds(t *testing.T) {
	nodes := []map[string]any{
		{"id": "oc1", "type": "CREDIT", "kind": "CONTRIBUTION", "createdAt": "2026-10-01T00:00:00Z",
			"amount":      map[string]any{"valueInCents": 2500, "currency": "USD"},
			"fromAccount": map[string]any{"name": "Donor", "slug": "donor"}},
		{"id": "oc2", "type": "CREDIT", "kind": "HOST_FEE_SHARE", "createdAt": "2026-10-01T00:00:00Z",
			"amount":      map[string]any{"valueInCents": 99, "currency": "USD"},
			"fromAccount": map[string]any{"name": "Host", "slug": "host"}},
	}
	srv := gqlServer(t, map[string]any{"account": map[string]any{
		"transactions": map[string]any{"totalCount": 2, "nodes": nodes},
	}})
	defer srv.Close()

	c := NewWithBaseURL(srv.URL, srv.Client())
	got, err := c.ListContributions(context.Background(), "collective", "tok", 100, 0)
	if err != nil {
		t.Fatalf("ListContributions: %v", err)
	}
	if len(got) != 1 || got[0].ID != "oc1" || got[0].AmountCents != 2500 || got[0].FromName != "Donor" {
		t.Fatalf("unexpected contributions: %+v", got)
	}
}

func TestListContributionsUnauthorized(t *testing.T) {
	srv := gqlServer(t, nil)
	defer srv.Close()
	c := NewWithBaseURL(srv.URL, srv.Client())
	if _, err := c.ListContributions(context.Background(), "x", "", 10, 0); err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}
