package stablecoin

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// DB-backed tests for the sqlc store seam — the fakes in stablecoin_test.go
// can't catch SQL-level regressions (COALESCE preservation, ON CONFLICT,
// aggregates). Skipped when no test DB is configured.

func testDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set — skipping DB integration test")
	}
	d, err := db.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("cannot connect to test DB: %v", err)
	}
	t.Cleanup(func() { d.Writer.Close(); d.Reader.Close() })
	return d
}

func newTestUser(t *testing.T, ctx context.Context, d *db.DB) pgtype.UUID {
	t.Helper()
	uname := fmt.Sprintf("sctest_%d", time.Now().UnixNano())
	var id pgtype.UUID
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email) VALUES ($1, $2) RETURNING id`,
		uname, uname+"@test.dev").Scan(&id); err != nil {
		t.Fatalf("inserting user: %v", err)
	}
	return id
}

func newTestWallet(t *testing.T, ctx context.Context, st *Store, user pgtype.UUID) *Wallet {
	t.Helper()
	w, err := st.InsertWallet(ctx, Wallet{
		UserID:  user.String(),
		Chain:   "solana",
		Address: fmt.Sprintf("addr-%d", time.Now().UnixNano()),
		Locator: fmt.Sprintf("loc-%d", time.Now().UnixNano()),
		Status:  WalletStatusActive,
	})
	if err != nil {
		t.Fatalf("InsertWallet: %v", err)
	}
	return w
}

// SetEscrowState must preserve funded_sig/decision/deadline when a
// transition supplies empty values — the regression the in-memory fake
// cannot catch (fake preserves by construction; the SQL needs COALESCE).
func TestDBSetEscrowStatePreservesFields(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	st := NewStore(gen.New(d.Writer))
	user := newTestUser(t, ctx, d)
	w := newTestWallet(t, ctx, st, user)

	// service_escrows.booking_id is a hard FK — reuse an existing booking
	// (seeding the contents→listing_meta→booking chain is out of scope).
	var bookingID string
	if err := d.Reader.QueryRow(ctx, `SELECT id::text FROM service_bookings LIMIT 1`).Scan(&bookingID); err != nil {
		t.Skipf("no service_bookings row in test DB: %v", err)
	}
	e, err := st.InsertEscrow(ctx, Escrow{
		BookingID:      bookingID,
		BuyerWalletID:  w.ID,
		SellerWalletID: w.ID,
		VaultAddress:   fmt.Sprintf("vault-%d", time.Now().UnixNano()),
		Asset:          "usdc",
		AmountMinor:    1000,
		State:          EscrowFunding,
	})
	if err != nil {
		t.Fatalf("InsertEscrow: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM service_escrows WHERE id = $1`, e.ID)
	})
	eid, _ := pgUUID(e.ID)
	deadline := time.Now().Add(72 * time.Hour).UTC()

	// Fund it — sig + deadline land.
	if err := st.SetEscrowState(ctx, eid, EscrowFunded, "fund-sig-1", "", nil, &deadline); err != nil {
		t.Fatal(err)
	}
	// Dispute — empty sig fields must NOT wipe funded_sig.
	if err := st.SetEscrowState(ctx, eid, EscrowDisputed, "", "", &ArbiterDecision{
		ReleasedTo: "", DecidedBy: "opener", DecidedAt: time.Now(), Note: "dispute: x",
	}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetEscrow(ctx, eid)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != EscrowDisputed || got.FundedSig != "fund-sig-1" {
		t.Fatalf("dispute wiped funded_sig: %+v", got)
	}
	if got.ConfirmationDeadline.IsZero() {
		t.Fatal("confirmation_deadline not persisted")
	}
	if got.ArbiterDecision == nil || got.ArbiterDecision.Note != "dispute: x" {
		t.Fatalf("decision not recorded: %+v", got.ArbiterDecision)
	}

	// Release — sig lands, funded_sig survives.
	if err := st.SetEscrowState(ctx, eid, EscrowReleased, "", "rel-sig-1", nil, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetEscrow(ctx, eid)
	if got.ReleaseSig != "rel-sig-1" || got.FundedSig != "fund-sig-1" {
		t.Fatalf("release wiped fields: %+v", got)
	}
}

// InsertActivity relies on ON CONFLICT (tx_signature) for webhook-replay
// safety — a second insert must not error or duplicate.
func TestDBInsertActivityDedupesOnSig(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	st := NewStore(gen.New(d.Writer))
	user := newTestUser(t, ctx, d)
	w := newTestWallet(t, ctx, st, user)

	a := Activity{
		WalletID: w.ID, Direction: "in", Kind: ActivityDeposit,
		Asset: "usdc", AmountMinor: 100, TxSignature: fmt.Sprintf("sig-%d", time.Now().UnixNano()),
		Status: "confirmed", OccurredAt: time.Now().UTC(),
	}
	if _, err := st.InsertActivity(ctx, a); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if _, err := st.InsertActivity(ctx, a); err != nil {
		t.Fatalf("replay insert errored — ON CONFLICT missing: %v", err)
	}
	var n int
	if err := d.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM stablecoin_activity WHERE tx_signature=$1`, a.TxSignature).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows=%d err=%v, want 1", n, err)
	}
}

// SumOutByAssetSince powers the delegated-signer daily-cap pre-check —
// pending+confirmed outflows count, failed and inbound don't.
func TestDBSumOutByAssetSince(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	st := NewStore(gen.New(d.Writer))
	user := newTestUser(t, ctx, d)
	w := newTestWallet(t, ctx, st, user)
	wid, _ := pgUUID(w.ID)
	since := time.Now().Add(-time.Hour).UTC()
	uniq := fmt.Sprintf("%d", time.Now().UnixNano())

	for i, row := range []struct {
		dir, kind, status, asset string
		amt                      int64
	}{
		{"out", ActivityWithdrawal, "confirmed", "usdc", 100},
		{"out", ActivityTipOut, "pending", "usdc", 50},
		{"out", ActivityWithdrawal, "failed", "usdc", 999},    // excluded
		{"in", ActivityDeposit, "confirmed", "usdc", 999},     // excluded
		{"out", ActivityWithdrawal, "confirmed", "usdt", 999}, // wrong asset
	} {
		_, err := st.InsertActivity(ctx, Activity{
			WalletID: w.ID, Direction: row.dir, Kind: row.kind,
			Asset: row.asset, AmountMinor: row.amt,
			TxSignature: fmt.Sprintf("sum-%s-%d", uniq, i),
			Status:      row.status, OccurredAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	got, err := st.SumOutByAssetSince(ctx, wid, "usdc", since)
	if err != nil {
		t.Fatal(err)
	}
	if got != 150 {
		t.Fatalf("sum = %d, want 150 (confirmed+pending out, usdc only)", got)
	}
}

// UpdateDelegatedSigner round-trips the jsonb signer record — scopes must
// survive the DB intact for the daily-cap pre-check.
func TestDBDelegatedSignerRoundtrip(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	st := NewStore(gen.New(d.Writer))
	user := newTestUser(t, ctx, d)
	w := newTestWallet(t, ctx, st, user)
	wid, _ := pgUUID(w.ID)

	exp := time.Now().Add(365 * 24 * time.Hour).UTC().Truncate(time.Second)
	ds := &DelegatedSigner{
		SignerRef: "signer-ref-1",
		Scopes: []SignerScope{
			{Asset: "usdc", DailyCapMinor: 5000, ExpiresAt: exp},
			{Asset: "usdt", DailyCapMinor: 2500, ExpiresAt: exp},
		},
		RegisteredAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := st.UpdateDelegatedSigner(ctx, wid, ds); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetWalletByID(ctx, wid)
	if err != nil {
		t.Fatal(err)
	}
	if got.DelegatedSigner == nil || len(got.DelegatedSigner.Scopes) != 2 {
		t.Fatalf("signer = %+v", got.DelegatedSigner)
	}
	if got.DelegatedSigner.Scopes[0].DailyCapMinor != 5000 {
		t.Fatalf("scope cap lost: %+v", got.DelegatedSigner.Scopes[0])
	}
}
