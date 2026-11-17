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
		*calls = append(*calls, req{method: r.Method, path: r.URL.Path, apiKey: r.Header.Get("X-API-KEY"), body: body})
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
			{Asset: "usdc", DailyCapMinor: 5000, ExpiresAt: exp},
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
	if limit["amount"] != "5000" || limit["interval"] != float64(86400) {
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
		"/transactions": `{"txSignature":"tx-abc","status":"submitted"}`,
	}, &calls)
	defer srv.Close()

	c := NewClient("k", srv.URL, "")
	res, err := c.Transfer(context.Background(), "loc-1", stablecoin.TransferInput{
		Asset: "usdc", RecipientAddr: "dest-1", AmountMinor: 1000,
		FeeAddr: "hot-1", FeeMinor: 50, Idempotency: "idem-1", Reference: "tip",
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
	p1 := calls[0].body["params"].(map[string]any)
	p2 := calls[1].body["params"].(map[string]any)
	if p1["destinationAddress"] != "dest-1" || p1["amount"] != "1000" || p1["idempotencyKey"] != "idem-1" {
		t.Fatalf("principal = %+v", p1)
	}
	if p2["destinationAddress"] != "hot-1" || p2["amount"] != "50" || p2["idempotencyKey"] != "idem-1:fee" {
		t.Fatalf("fee leg = %+v", p2)
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
		"/transactions": `{"id":"op-1","status":"awaiting-approval"}`,
	}, &calls)
	defer srv.Close()

	c := NewClient("k", srv.URL, "")
	op, err := c.BuildWithdrawal(context.Background(), "loc-1", stablecoin.WithdrawalInput{
		Asset: "usdc", Destination: "ext-addr", AmountMinor: 2000, FeeMinor: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if op.Locator != "loc-1" || len(op.Payload) == 0 || op.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("op = %+v", op)
	}
	if calls[0].body["requiresApproval"] != true {
		t.Fatalf("member-signed op not flagged: %+v", calls[0].body)
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
	events, err := c.Activity(context.Background(), "loc-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].TxSignature != "s1" || events[0].AmountMinor != 500 || events[0].OccurredAt != 1700000000 {
		t.Fatalf("events = %+v", events)
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
