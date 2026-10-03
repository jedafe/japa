// Package fx — handlers.go provides the HTTP handlers for the FX Currency
// Converter routes (Section 112, ADR-125).
//
// Public routes (no auth), gated by the fx.enabled feature flag:
//
//	GET /api/v1/fx/convert     — ?amount=&from=&to=
//	GET /api/v1/fx/rates       — ?base=USD (defaults to fx.default_base_currency)
//	GET /api/v1/fx/currencies  — picker list
//
// Ref: japaconnect-engineering/sections/112-fx-converter.md
//
//	japaconnect-engineering/adr/ADR-125-fx-converter.md
package fx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// serviceAPI is the slice of *Service behaviour the handlers depend on.
// Declared as an interface so handler tests can stub service responses
// without a database.
type serviceAPI interface {
	Convert(ctx context.Context, amount float64, from, to string) (*ConvertResult, error)
	GetRates(ctx context.Context, base string) (*Matrix, bool, error)
	ListCurrencies(ctx context.Context) ([]gen.ListFxCurrenciesRow, error)
	DefaultBaseCurrency(ctx context.Context) string
}

// Handlers exposes the FX converter HTTP endpoints.
type Handlers struct {
	service serviceAPI
}

// NewHandlers creates fx handlers bound to the provided service.
func NewHandlers(service *Service) *Handlers {
	if service == nil {
		return &Handlers{}
	}

	return &Handlers{service: service}
}

// writeServiceErr maps service errors to the standard error envelope (Rule 5).
func writeServiceErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUnsupportedCurrency):
		errpkg.WriteError(w, http.StatusBadRequest, "unsupported_currency", "Currency not supported")
	case errors.Is(err, ErrEmptyMatrix):
		errpkg.WriteError(w, http.StatusServiceUnavailable, "fx_unavailable", "FX rates are not available yet")
	default:
		slog.Error("fx: handler failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "FX request failed")
	}
}

// isISO4217 reports whether s looks like a 3-letter ISO 4217 currency code.
func isISO4217(s string) bool {
	if len(s) != 3 {
		return false
	}

	for _, c := range s {
		if c < 'A' || c > 'Z' {
			return false
		}
	}

	return true
}

// HandleConvert handles GET /api/v1/fx/convert — ?amount=&from=&to=.
func (h *Handlers) HandleConvert(w http.ResponseWriter, r *http.Request) {
	amountStr := r.URL.Query().Get("amount")
	from := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("from")))
	to := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("to")))

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_amount", "amount must be a positive number")
		return
	}

	if !isISO4217(from) || !isISO4217(to) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_currency", "from and to must be 3-letter ISO 4217 currency codes")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "fx_unavailable", "FX service not configured")
		return
	}

	result, err := h.service.Convert(r.Context(), amount, from, to)
	if err != nil {
		writeServiceErr(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// HandleGetRates handles GET /api/v1/fx/rates — ?base=USD (defaults to the
// configured fx.default_base_currency when omitted).
func (h *Handlers) HandleGetRates(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "fx_unavailable", "FX service not configured")
		return
	}

	base := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("base")))
	if base == "" {
		base = h.service.DefaultBaseCurrency(r.Context())
	}

	m, stale, err := h.service.GetRates(r.Context(), base)
	if err != nil {
		writeServiceErr(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"base":       m.Base,
		"rates":      m.Rates,
		"fetched_at": m.FetchedAt.UTC().Format(time.RFC3339),
		"stale":      stale,
	}})
}

// HandleListCurrencies handles GET /api/v1/fx/currencies — picker list of
// active currencies that have matrix coverage.
func (h *Handlers) HandleListCurrencies(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "fx_unavailable", "FX service not configured")
		return
	}

	rows, err := h.service.ListCurrencies(r.Context())
	if err != nil {
		writeServiceErr(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}
