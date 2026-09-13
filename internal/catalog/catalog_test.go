package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestGetCatalog(t *testing.T) {
	// Build a minimal router with the catalog route, injecting a valid tenant.
	r := chi.NewRouter()
	h := NewHandlers()
	r.Get("/catalogs/{name}", h.GetCatalog)

	tests := []struct {
		name         string
		catalog      string
		wantStatus   int
		wantLen      int
		wantFirst    CatalogOption
		wantNotFound bool
	}{
		{
			name:       "tax_calculator.employment_status",
			catalog:    "tax_calculator.employment_status",
			wantStatus: http.StatusOK,
			wantLen:    5,
			wantFirst:  CatalogOption{Value: "", Label: "Any (optional)"},
		},
		{
			name:       "remittance.alert_direction",
			catalog:    "remittance.alert_direction",
			wantStatus: http.StatusOK,
			wantLen:    2,
			wantFirst:  CatalogOption{Value: "above", Label: "rate goes above"},
		},
		{
			name:       "cv_formatter.format",
			catalog:    "cv_formatter.format",
			wantStatus: http.StatusOK,
			wantLen:    2,
			wantFirst:  CatalogOption{Value: "docx", Label: "DOCX"},
		},
		{
			name:       "appointment_alert.notification_channels",
			catalog:    "appointment_alert.notification_channels",
			wantStatus: http.StatusOK,
			wantLen:    4,
			wantFirst:  CatalogOption{Value: "in_app", Label: "In-app", Default: true},
		},
		{
			name:       "scholarships.renewable",
			catalog:    "scholarships.renewable",
			wantStatus: http.StatusOK,
			wantLen:    3,
			wantFirst:  CatalogOption{Value: "", Label: "Any"},
		},
		{
			name:         "unknown catalog",
			catalog:      "unknown.catalog",
			wantStatus:   http.StatusNotFound,
			wantNotFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/catalogs/"+tt.catalog, nil)
			// Simulate tenant middleware by injecting a valid tenant.
			tctx := context.WithValue(
				req.Context(),
				struct{}{},
				&struct {
					ID   pgtype.UUID
					Name string
					Slug string
				}{ID: pgtype.UUID{Valid: true}},
			)
			req = req.WithContext(tctx)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}

			if tt.wantNotFound {
				return
			}

			var body struct {
				Data []CatalogOption `json:"data"`
			}

			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if len(body.Data) != tt.wantLen {
				t.Fatalf("len(data) = %d, want %d", len(body.Data), tt.wantLen)
			}

			if body.Data[0] != tt.wantFirst {
				t.Fatalf("first option = %+v, want %+v", body.Data[0], tt.wantFirst)
			}

			// Ensure the returned slice is a copy, not the shared catalog.
			body.Data[0].Value = "mutated"

			if publicCatalogs[tt.catalog][0].Value == "mutated" {
				t.Fatal("handler returned a mutable reference to the shared catalog")
			}
		})
	}
}
