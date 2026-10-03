// Package fx — handlers_test.go tests the FX converter HTTP handlers:
// query-param validation and the sentinel→HTTP error mapping (Rule 5).
// A stub serviceAPI covers service-layer behaviour without a database.
//
// Ref: japaconnect-engineering/sections/112-fx-converter.md
//
//	japaconnect-engineering/adr/ADR-125-fx-converter.md
package fx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jedafe/japa/internal/db/gen"
)

// stubService implements serviceAPI for handler tests.
type stubService struct {
	convertResult *ConvertResult
	convertErr    error
	matrix        *Matrix
	stale         bool
	ratesErr      error
	currencies    []gen.ListFxCurrenciesRow
	currenciesErr error
	defaultBase   string
}

func (s *stubService) Convert(_ context.Context, amount float64, from, to string) (*ConvertResult, error) {
	return s.convertResult, s.convertErr
}

func (s *stubService) GetRates(_ context.Context, _ string) (*Matrix, bool, error) {
	return s.matrix, s.stale, s.ratesErr
}

func (s *stubService) ListCurrencies(_ context.Context) ([]gen.ListFxCurrenciesRow, error) {
	return s.currencies, s.currenciesErr
}

func (s *stubService) DefaultBaseCurrency(_ context.Context) string {
	if s.defaultBase == "" {
		return "USD"
	}

	return s.defaultBase
}

// errCode extracts the machine-readable code from the error envelope.
func errCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not the error envelope: %v (body: %s)", err, rr.Body.String())
	}

	return body.Error.Code
}

func TestHandleConvert_Validation(t *testing.T) {
	h := &Handlers{service: &stubService{}}

	tests := []struct {
		name     string
		url      string
		wantCode string
	}{
		{"missing amount", "/fx/convert?from=USD&to=NGN", "invalid_amount"},
		{"non-numeric amount", "/fx/convert?amount=abc&from=USD&to=NGN", "invalid_amount"},
		{"zero amount", "/fx/convert?amount=0&from=USD&to=NGN", "invalid_amount"},
		{"negative amount", "/fx/convert?amount=-5&from=USD&to=NGN", "invalid_amount"},
		{"missing from", "/fx/convert?amount=100&to=NGN", "invalid_currency"},
		{"missing to", "/fx/convert?amount=100&from=USD", "invalid_currency"},
		{"two-letter code", "/fx/convert?amount=100&from=US&to=NGN", "invalid_currency"},
		{"four-letter code", "/fx/convert?amount=100&from=USD&to=USDT", "invalid_currency"},
		{"numeric code", "/fx/convert?amount=100&from=123&to=NGN", "invalid_currency"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rr := httptest.NewRecorder()
			h.HandleConvert(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusBadRequest, rr.Body.String())
			}
			if got := errCode(t, rr); got != tc.wantCode {
				t.Fatalf("error code = %q, want %q", got, tc.wantCode)
			}
		})
	}
}

func TestHandleConvert_Success(t *testing.T) {
	h := &Handlers{service: &stubService{convertResult: &ConvertResult{
		Amount: 100, From: "USD", To: "NGN", Rate: 1600, Converted: 160000,
		FetchedAt: time.Now().UTC().Format(time.RFC3339), Stale: false,
	}}}

	req := httptest.NewRequest(http.MethodGet, "/fx/convert?amount=100&from=usd&to=ngn", nil)
	rr := httptest.NewRecorder()
	h.HandleConvert(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}

	var body struct {
		Data ConvertResult `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Data.Converted != 160000 || body.Data.From != "USD" || body.Data.To != "NGN" {
		t.Fatalf("unexpected convert payload: %+v", body.Data)
	}
}

func TestHandleConvert_ServiceErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"empty matrix", ErrEmptyMatrix, http.StatusServiceUnavailable, "fx_unavailable"},
		{"unsupported currency", ErrUnsupportedCurrency, http.StatusBadRequest, "unsupported_currency"},
		{"wrapped sentinel", errors.Join(errors.New("db down"), ErrEmptyMatrix), http.StatusServiceUnavailable, "fx_unavailable"},
		{"unexpected", errors.New("boom"), http.StatusInternalServerError, "internal_error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handlers{service: &stubService{convertErr: tc.err}}
			req := httptest.NewRequest(http.MethodGet, "/fx/convert?amount=100&from=USD&to=NGN", nil)
			rr := httptest.NewRecorder()
			h.HandleConvert(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if got := errCode(t, rr); got != tc.wantCode {
				t.Fatalf("error code = %q, want %q", got, tc.wantCode)
			}
		})
	}
}

func TestHandleGetRates_DefaultBase(t *testing.T) {
	stub := &stubService{
		matrix: &Matrix{Base: "USD", Rates: map[string]float64{"USD": 1, "NGN": 1600}, FetchedAt: time.Now()},
	}
	h := &Handlers{service: stub}

	// No base param → service default (fx.default_base_currency fallback "USD").
	req := httptest.NewRequest(http.MethodGet, "/fx/rates", nil)
	rr := httptest.NewRecorder()
	h.HandleGetRates(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}

	var body struct {
		Data struct {
			Base      string             `json:"base"`
			Rates     map[string]float64 `json:"rates"`
			FetchedAt string             `json:"fetched_at"`
			Stale     bool               `json:"stale"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Data.Base != "USD" || body.Data.Rates["NGN"] != 1600 {
		t.Fatalf("unexpected rates payload: %+v", body.Data)
	}
}

func TestHandleGetRates_EmptyMatrix(t *testing.T) {
	h := &Handlers{service: &stubService{ratesErr: ErrEmptyMatrix}}
	req := httptest.NewRequest(http.MethodGet, "/fx/rates?base=EUR", nil)
	rr := httptest.NewRecorder()
	h.HandleGetRates(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
	if got := errCode(t, rr); got != "fx_unavailable" {
		t.Fatalf("error code = %q, want fx_unavailable", got)
	}
}

func TestHandleGetRates_UnsupportedBase(t *testing.T) {
	h := &Handlers{service: &stubService{ratesErr: ErrUnsupportedCurrency}}
	req := httptest.NewRequest(http.MethodGet, "/fx/rates?base=XXX", nil)
	rr := httptest.NewRecorder()
	h.HandleGetRates(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if got := errCode(t, rr); got != "unsupported_currency" {
		t.Fatalf("error code = %q, want unsupported_currency", got)
	}
}

func TestHandleListCurrencies(t *testing.T) {
	h := &Handlers{service: &stubService{currencies: []gen.ListFxCurrenciesRow{
		{Code: "USD", Name: "US Dollar", Symbol: "$"},
		{Code: "NGN", Name: "Nigerian Naira", Symbol: "₦"},
	}}}

	req := httptest.NewRequest(http.MethodGet, "/fx/currencies", nil)
	rr := httptest.NewRecorder()
	h.HandleListCurrencies(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}

	var body struct {
		Data []gen.ListFxCurrenciesRow `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Data) != 2 || body.Data[0].Code != "USD" {
		t.Fatalf("unexpected currencies payload: %+v", body.Data)
	}
}

func TestHandlers_NilService(t *testing.T) {
	h := NewHandlers(nil)

	tests := []struct {
		name    string
		url     string
		handler http.HandlerFunc
	}{
		{"convert", "/fx/convert?amount=100&from=USD&to=NGN", h.HandleConvert},
		{"rates", "/fx/rates", h.HandleGetRates},
		{"currencies", "/fx/currencies", h.HandleListCurrencies},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rr := httptest.NewRecorder()
			tc.handler(rr, req)

			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
			}
			if got := errCode(t, rr); got != "fx_unavailable" {
				t.Fatalf("error code = %q, want fx_unavailable", got)
			}
		})
	}
}
