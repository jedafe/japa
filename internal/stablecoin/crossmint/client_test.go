package crossmint

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jedafe/japa/internal/stablecoin"
)

// req captures one inbound call for assertion.
type req struct {
	method string
	path   string
	apiKey string
	idem   string
	body   map[string]any
}

// server stubs the Crossmint API, records requests, and replays canned
// responses keyed by path suffix.
func server(t *testing.T, responses map[string]string, calls *[]req) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		body := map[string]any{}
		if len(data) > 0 {
			_ = json.Unmarshal(data, &body)
		}
		*calls = append(*calls, req{method: r.Method, path: r.URL.Path, apiKey: r.Header.Get("X-API-KEY"), idem: r.Header.Get("x-idempotency-key"), body: body})
		for suffix, resp := range responses {
			if len(r.URL.Path) >= len(suffix) && r.URL.Path[len(r.URL.Path)-len(suffix):] == suffix {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(resp))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
}

func TestCreateWalletShape(t *testing.T) {
	var calls []req
	srv := server(t, map[string]string{
		"/wallets": `{"address":"addr-9","locator":"loc-9","chain":"solana","type":"smart","status":"active"}`,
	}, &calls)
	defer srv.Close()

	c := NewClient("key-1", srv.URL, "")
	w, err := c.CreateWallet(context.Background(), stablecoin.CreateWalletInput{
		UserID: "u-1", Chain: "solana", RecoveryMethods: []string{"email"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if w.Address != "addr-9" || w.Locator != "loc-9" {
		t.Fatalf("wallet = %+v", w)
	}
	if len(calls) != 1 || calls[0].method != "POST" || calls[0].path != walletsBase {
		t.Fatalf("call = %+v", calls)
	}
	if calls[0].apiKey != "key-1" {
		t.Fatalf("missing X-API-KEY header: %q", calls[0].apiKey)
	}
	if calls[0].body["type"] != "smart" || calls[0].body["chain"] != "solana" || calls[0].body["linkedUser"] != "userId:u-1" {
		t.Fatalf("body = %+v", calls[0].body)
	}
}

func TestRegisterDelegatedSignerScopes(t *testing.T) {
	var calls []req
	srv := server(t, map[string]string{"/signers": `{}`}, &calls)
	defer srv.Close()

	exp := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	c := NewClient("k", srv.URL, "")
	err := c.RegisterDelegatedSigner(context.Background(), "loc-1", stablecoin.DelegatedSignerInput{
		SignerRef: "platform-signer-pubkey",
		Scopes: []stablecoin.SignerScope{
			{Asset: "usdc", DailyCapMinor: 5_000_000, ExpiresAt: exp},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls[0].method != "POST" || calls[0].path != walletsBase+"/loc-1/signers" {
		t.Fatalf("call = %+v", calls[0])
	}
	scopes, ok := calls[0].body["scopes"].([]any)
	if !ok || len(scopes) != 1 {
		t.Fatalf("scopes = %+v", calls[0].body["scopes"])
	}
	scope := scopes[0].(map[string]any)
	if scope["type"] != "transfer" || scope["tokenLocator"] != "solana:usdc" {
		t.Fatalf("scope = %+v", scope)
	}
	limit := scope["spendingLimit"].(map[string]any)
	// Display units per documented API — 5_000_000 minor must wire as "5",
	// not "5000000" (raw minor would inflate the onchain cap by 10^6).
	if limit["amount"] != "5" || limit["interval"] != float64(86400) {
		t.Fatalf("spendingLimit = %+v", limit)
	}
	if calls[0].body["expiresAt"] == nil {
		t.Fatal("expiresAt missing — signer would never expire")
	}
	signer := calls[0].body["signer"].(map[string]any)
	if signer["address"] != "platform-signer-pubkey" {
		t.Fatalf("signer = %+v", signer)
	}
}

func TestTransferFeeLegSequencing(t *testing.T) {
	var calls []req
	srv := server(t, map[string]string{
		"/transfers": `{"txSignature":"tx-abc","status":"submitted"}`,
	}, &calls)
	defer srv.Close()

	c := NewClient("k", srv.URL, "")
	res, err := c.Transfer(context.Background(), "loc-1", stablecoin.TransferInput{
		Asset: "usdc", RecipientAddr: "dest-1", AmountMinor: 1_000_000,
		FeeAddr: "hot-1", FeeMinor: 50_000, Idempotency: "idem-1", Reference: "tip",
		Signer: "signer-pubkey",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.TxSignature != "tx-abc" {
		t.Fatalf("result = %+v", res)
	}
	if len(calls) != 2 {
		t.Fatalf("transfers = %d, want principal+fee", len(calls))
	}
	// Documented wire: /tokens/{tokenLocator}/transfers, display-unit amount,
	// `recipient`, delegated signer as "external-wallet:<addr>", and the
	// idempotency key on the x-idempotency-key HEADER — not the body.
	if calls[0].path != walletsBase+"/loc-1/tokens/solana:usdc/transfers" {
		t.Fatalf("path = %s", calls[0].path)
	}
	if calls[0].body["recipient"] != "dest-1" || calls[0].body["amount"] != "1" ||
		calls[0].body["signer"] != "external-wallet:signer-pubkey" || calls[0].idem != "idem-1" {
		t.Fatalf("principal = %+v", calls[0])
	}
	if calls[1].body["recipient"] != "hot-1" || calls[1].body["amount"] != "0.05" || calls[1].idem != "idem-1:fee" {
		t.Fatalf("fee leg = %+v", calls[1])
	}
}

func TestTransferFeeLegFailureKeepsPrincipalVisible(t *testing.T) {
	// First transaction succeeds, second 500s — keyed by which body arrives.
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			w.Write([]byte(`{"txSignature":"tx-ok"}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, "")
	res, err := c.Transfer(context.Background(), "loc-1", stablecoin.TransferInput{
		Asset: "usdc", RecipientAddr: "d", AmountMinor: 100,
		FeeAddr: "hot", FeeMinor: 5, Idempotency: "i",
	})
	if err == nil {
		t.Fatal("fee-leg failure should surface an error")
	}
	if res.TxSignature != "tx-ok" {
		t.Fatalf("principal result lost on fee failure: %+v", res)
	}
}

func TestBuildWithdrawalRequiresApproval(t *testing.T) {
	var calls []req
	srv := server(t, map[string]string{
		"/transfers": `{"id":"op-1","status":"awaiting-approval"}`,
	}, &calls)
	defer srv.Close()

	c := NewClient("k", srv.URL, "")
	op, err := c.BuildWithdrawal(context.Background(), "loc-1", stablecoin.WithdrawalInput{
		Asset: "usdc", Destination: "ext-addr", AmountMinor: 2_000_000, FeeMinor: 20_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if op.Locator != "loc-1" || len(op.Payload) == 0 || op.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("op = %+v", op)
	}
	// Member-signed op: transfers endpoint, display units, no delegated
	// signer field — the member's own approval flow is what signs.
	if calls[0].body["amount"] != "2" || calls[0].body["recipient"] != "ext-addr" || calls[0].body["signer"] != nil {
		t.Fatalf("member op = %+v", calls[0].body)
	}
}

func TestGetWalletNormalizesBalances(t *testing.T) {
	var calls []req
	srv := server(t, map[string]string{
		"/loc-1": `{"address":"a","chain":"solana","status":"active","balances":[{"token":"solana:usdc","amount":"12345"},{"token":"solana:usdt","amount":"7"}]}`,
	}, &calls)
	defer srv.Close()

	c := NewClient("k", srv.URL, "")
	w, err := c.GetWallet(context.Background(), "loc-1")
	if err != nil {
		t.Fatal(err)
	}
	if w.Balances["usdc"] != 12345 || w.Balances["usdt"] != 7 {
		t.Fatalf("balances = %+v", w.Balances)
	}
}

func TestActivityDecoding(t *testing.T) {
	var calls []req
	srv := server(t, map[string]string{
		"/activity": `{"events":[{"type":"transfer_in","txSignature":"s1","token":"solana:usdc","amount":"500","timestamp":1700000000,"counterparty":"cp-1"}]}`,
	}, &calls)
	defer srv.Close()

	c := NewClient("k", srv.URL, "")
	events, next, err := c.Activity(context.Background(), "loc-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if next != "" {
		t.Fatalf("next = %q, want terminal page", next)
	}
	if len(events) != 1 || events[0].TxSignature != "s1" || events[0].AmountMinor != 500 || events[0].OccurredAt != 1700000000 {
		t.Fatalf("events = %+v", events)
	}
}

func TestActivityPaginationAndMsTimestamp(t *testing.T) {
	var calls []req
	srv := server(t, map[string]string{
		"/activity": `{"events":[{"type":"transfer_in","txSignature":"s2","token":"usdc","amount":"1","timestamp":1700000000000}],"nextCursor":"c-2"}`,
	}, &calls)
	defer srv.Close()

	c := NewClient("k", srv.URL, "")
	events, next, err := c.Activity(context.Background(), "loc-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if next != "c-2" {
		t.Fatalf("next = %q, want c-2", next)
	}
	if events[0].OccurredAt != 1700000000 { // ms timestamp must normalize to seconds
		t.Fatalf("occurred = %d, want 1700000000", events[0].OccurredAt)
	}
}

func TestVerifyWebhook(t *testing.T) {
	secret := "whsec-1"
	c := NewClient("k", "http://x", secret)
	payload := []byte(`{"type":"transfer_in","tx_signature":"s1"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	good := hex.EncodeToString(mac.Sum(nil))

	if err := c.VerifyWebhook(payload, good); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := c.VerifyWebhook(payload, "sha256="+good); err != nil {
		t.Fatalf("prefixed signature rejected: %v", err)
	}
	if err := c.VerifyWebhook(payload, "deadbeef"); err == nil {
		t.Fatal("bad signature accepted")
	}
	if err := c.VerifyWebhook(payload, ""); err == nil {
		t.Fatal("missing signature accepted")
	}
}
