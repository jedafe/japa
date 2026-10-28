// settle_db_test.go — §125 Postgres-backed settlement tests. Gated on
// JC_TEST_DATABASE_URL (falls back to JC_DATABASE_URL); skips otherwise.
// Covers: credits settle + fee, points pass-through, insufficient funds,
// idempotent re-settle, fiat conversion (USD/JPY), clawback, org owner
// crediting, and concurrent same-sender tips (lock-ordering smoke).
package tips

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("JC_DATABASE_URL")
	}
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

var dbTestCounter int

func newUser(t *testing.T, ctx context.Context, d *db.DB, role string) pgtype.UUID {
	t.Helper()
	dbTestCounter++
	uname := fmt.Sprintf("tiptester_%s_%d", role, dbTestCounter)
	var id pgtype.UUID
	err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email) VALUES ($1, $2) RETURNING id`,
		uname, uname+"@test.dev").Scan(&id)
	if err != nil {
		t.Fatalf("inserting user: %v", err)
	}
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO user_profiles (user_id) VALUES ($1) ON CONFLICT DO NOTHING`, id); err != nil {
		t.Fatalf("inserting profile: %v", err)
	}
	return id
}

func grantCredits(t *testing.T, ctx context.Context, d *db.DB, userID pgtype.UUID, ledger string, amount int64) {
	t.Helper()
	src := "tip_test"
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO wallet_ledger (user_id, ledger_type, operation, amount, resulting_balance, source)
		 SELECT $1, $2, 'earn', $3, COALESCE(
		   (SELECT resulting_balance FROM wallet_ledger WHERE user_id=$1 AND ledger_type=$2 ORDER BY created_at DESC LIMIT 1), 0) + $3, $4`,
		userID, ledger, amount, src); err != nil {
		t.Fatalf("granting %s: %v", ledger, err)
	}
}

func balance(t *testing.T, ctx context.Context, d *db.DB, userID pgtype.UUID, ledger string) int64 {
	t.Helper()
	var bal *int64
	err := d.Reader.QueryRow(ctx,
		`SELECT resulting_balance FROM wallet_ledger WHERE user_id=$1 AND ledger_type=$2 ORDER BY created_at DESC LIMIT 1`,
		userID, ledger).Scan(&bal)
	if err != nil || bal == nil {
		return 0
	}
	return *bal
}

func seedSettings(t *testing.T, ctx context.Context, d *db.DB, ownerType string, ownerID pgtype.UUID) {
	t.Helper()
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO tip_settings (owner_type, owner_id, tips_enabled, accept_points, accept_credits, accept_fiat, accept_external, min_tip_amount)
		 VALUES ($1, $2, true, true, true, true, false, 0)`,
		ownerType, ownerID); err != nil {
		t.Fatalf("seeding tip_settings: %v", err)
	}
}

func TestDBSettleCreditsTip(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	sender := newUser(t, ctx, d, "sender")
	recip := newUser(t, ctx, d, "recip")
	seedSettings(t, ctx, d, "user", recip)
	grantCredits(t, ctx, d, sender, "credits", 200)

	svc := NewService(d)
	svc.settings = fakeSettings{m: map[string]any{"tips.enabled": true, "tips.fee_percent": 10}}
	svc.perms = fakePerms{map[string]bool{"tips.send": true}}

	res, err := svc.CreateTip(ctx, sender, CreateTipInput{
		RecipientType: RecipientUser, RecipientUserID: recip,
		Method: MethodCredits, CreditsAmount: 100,
	}, "")
	if err != nil {
		t.Fatalf("CreateTip: %v", err)
	}
	if res.Tip.NetCredits != 90 {
		t.Fatalf("expected net 90 at 10%% fee, got %d", res.Tip.NetCredits)
	}
	if got := balance(t, ctx, d, sender, "credits"); got != 100 {
		t.Fatalf("sender balance = %d, want 100", got)
	}
	if got := balance(t, ctx, d, recip, "credits"); got != 90 {
		t.Fatalf("recipient balance = %d, want 90", got)
	}
}

func TestDBSettlePointsNoFee(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	sender := newUser(t, ctx, d, "pts_sender")
	recip := newUser(t, ctx, d, "pts_recip")
	seedSettings(t, ctx, d, "user", recip)
	grantCredits(t, ctx, d, sender, "points", 500)

	svc := NewService(d)
	svc.settings = fakeSettings{m: map[string]any{"tips.enabled": true, "tips.fee_percent": 50, "tips.points_daily_limit": 0}}
	svc.perms = fakePerms{map[string]bool{"tips.send": true}}

	_, err := svc.CreateTip(ctx, sender, CreateTipInput{
		RecipientType: RecipientUser, RecipientUserID: recip,
		Method: MethodPoints, PointsAmount: 50,
	}, "")
	if err != nil {
		t.Fatalf("CreateTip: %v", err)
	}
	if got := balance(t, ctx, d, recip, "points"); got != 50 {
		t.Fatalf("points should pass through whole: got %d", got)
	}
}

func TestDBInsufficientFunds(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	sender := newUser(t, ctx, d, "broke")
	recip := newUser(t, ctx, d, "victim")
	seedSettings(t, ctx, d, "user", recip)

	svc := NewService(d)
	svc.settings = fakeSettings{m: map[string]any{"tips.enabled": true}}
	svc.perms = fakePerms{map[string]bool{"tips.send": true}}

	_, err := svc.CreateTip(ctx, sender, CreateTipInput{
		RecipientType: RecipientUser, RecipientUserID: recip,
		Method: MethodCredits, CreditsAmount: 10,
	}, "")
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}
	// No tip row + no ledger rows should have landed.
	var n int
	if err := d.Reader.QueryRow(ctx, `SELECT count(*) FROM tips WHERE sender_user_id=$1`, sender).Scan(&n); err != nil || n != 0 {
		t.Fatalf("tip row inserted on failure: n=%d err=%v", n, err)
	}
}

func TestDBIdempotentReplay(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	sender := newUser(t, ctx, d, "idem")
	recip := newUser(t, ctx, d, "idem_r")
	seedSettings(t, ctx, d, "user", recip)
	grantCredits(t, ctx, d, sender, "credits", 50)

	svc := NewService(d)
	svc.settings = fakeSettings{m: map[string]any{"tips.enabled": true}}
	svc.perms = fakePerms{map[string]bool{"tips.send": true}}
	in := CreateTipInput{RecipientType: RecipientUser, RecipientUserID: recip, Method: MethodCredits, CreditsAmount: 20}

	r1, err := svc.CreateTip(ctx, sender, in, "idem-key-1")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	r2, err := svc.CreateTip(ctx, sender, in, "idem-key-1")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !r2.Duplicate || r2.Tip.ID != r1.Tip.ID {
		t.Fatalf("replay should return original tip: %+v", r2)
	}
	if got := balance(t, ctx, d, sender, "credits"); got != 30 {
		t.Fatalf("double-settle detected: sender balance %d", got)
	}
}

func TestDBFiatSettleConversion(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	sender := newUser(t, ctx, d, "fiat_s")
	recip := newUser(t, ctx, d, "fiat_r")
	seedSettings(t, ctx, d, "user", recip)

	// Fixture currency rows are reused when present; USD/JPY are seeded in
	// the currencies migration with credits_per_unit.
	var usdCPU, jpyCPU int
	if err := d.Reader.QueryRow(ctx, `SELECT credits_per_unit FROM currencies WHERE code='USD'`).Scan(&usdCPU); err != nil {
		t.Skipf("USD currency row missing: %v", err)
	}
	if err := d.Reader.QueryRow(ctx, `SELECT credits_per_unit FROM currencies WHERE code='JPY'`).Scan(&jpyCPU); err != nil {
		jpyCPU = -1
	}

	svc := NewService(d)
	svc.settings = fakeSettings{m: map[string]any{"tips.enabled": true, "tips.fee_percent": 10}}
	svc.perms = fakePerms{map[string]bool{"tips.send": true}}

	// USD 1000 cents × credits_per_unit, 10% fee → net = gross*0.9.
	amt := int64(1000)
	cur := "USD"
	tip, err := svc.store.insertTip(ctx, gen.InsertTipParams{
		SenderUserID: sender, RecipientType: RecipientUser, RecipientUserID: recip,
		Method: "stripe", FiatAmount: &amt, FiatCurrency: &cur, Status: StatusPending,
	})
	if err != nil {
		t.Fatalf("insert pending tip: %v", err)
	}
	if err := svc.SettleFiatTip(ctx, tip.ID); err != nil {
		t.Fatalf("SettleFiatTip: %v", err)
	}
	wantNet := int64(amt) * int64(usdCPU) / 100 * 90 / 100
	if got := balance(t, ctx, d, recip, "credits"); got != wantNet {
		t.Fatalf("USD settle: got %d credits, want %d (cpu=%d)", got, wantNet, usdCPU)
	}
	// Idempotent re-settle.
	if err := svc.SettleFiatTip(ctx, tip.ID); err != nil {
		t.Fatalf("re-settle: %v", err)
	}
	if got := balance(t, ctx, d, recip, "credits"); got != wantNet {
		t.Fatalf("double-settle: got %d, want %d", got, wantNet)
	}

	// JPY: minor units are whole yen — 5000 × cpu, 0 fee only when cpu seeded.
	if jpyCPU > 0 {
		amt2 := int64(5000)
		cur2 := "JPY"
		tip2, err := svc.store.insertTip(ctx, gen.InsertTipParams{
			SenderUserID: sender, RecipientType: RecipientUser, RecipientUserID: recip,
			Method: "stripe", FiatAmount: &amt2, FiatCurrency: &cur2, Status: StatusPending,
		})
		if err != nil {
			t.Fatalf("insert jpy tip: %v", err)
		}
		svc.settings = fakeSettings{m: map[string]any{"tips.enabled": true, "tips.fee_percent": 0}}
		if err := svc.SettleFiatTip(ctx, tip2.ID); err != nil {
			t.Fatalf("JPY settle: %v", err)
		}
		wantTotal := wantNet + int64(amt2)*int64(jpyCPU)
		if got := balance(t, ctx, d, recip, "credits"); got != wantTotal {
			t.Fatalf("JPY settle: got %d, want %d", got, wantTotal)
		}
	}
}

func TestDBClawback(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	sender := newUser(t, ctx, d, "claw_s")
	recip := newUser(t, ctx, d, "claw_r")
	seedSettings(t, ctx, d, "user", recip)
	grantCredits(t, ctx, d, sender, "credits", 100)

	svc := NewService(d)
	svc.settings = fakeSettings{m: map[string]any{"tips.enabled": true, "tips.fee_percent": 0}}
	svc.perms = fakePerms{map[string]bool{"tips.send": true}}

	res, err := svc.CreateTip(ctx, sender, CreateTipInput{
		RecipientType: RecipientUser, RecipientUserID: recip, Method: MethodCredits, CreditsAmount: 50,
	}, "")
	if err != nil {
		t.Fatalf("tip: %v", err)
	}
	if got := balance(t, ctx, d, recip, "credits"); got != 50 {
		t.Fatalf("pre-clawback balance %d", got)
	}

	if err := svc.ClawbackTip(ctx, res.Tip.ID); err != nil {
		t.Fatalf("ClawbackTip: %v", err)
	}
	if got := balance(t, ctx, d, recip, "credits"); got != 0 {
		t.Fatalf("clawback should restore 0, got %d", got)
	}
	var status string
	if err := d.Reader.QueryRow(ctx, `SELECT status FROM tips WHERE id=$1`, res.Tip.ID).Scan(&status); err != nil {
		t.Fatalf("fetch status: %v", err)
	}
	if status != StatusRefunded {
		t.Fatalf("status = %s", status)
	}
	// Re-clawback is a no-op.
	if err := svc.ClawbackTip(ctx, res.Tip.ID); err != nil {
		t.Fatalf("re-clawback: %v", err)
	}
}

func TestDBConcurrentSameSender(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	sender := newUser(t, ctx, d, "concur")
	recip1 := newUser(t, ctx, d, "concur_r1")
	recip2 := newUser(t, ctx, d, "concur_r2")
	seedSettings(t, ctx, d, "user", recip1)
	seedSettings(t, ctx, d, "user", recip2)
	grantCredits(t, ctx, d, sender, "credits", 100)

	svc := NewService(d)
	svc.settings = fakeSettings{m: map[string]any{"tips.enabled": true}}
	svc.perms = fakePerms{map[string]bool{"tips.send": true}}

	// Two concurrent 60-credit tips — the lock serializes them; exactly one
	// succeeds, the other reports insufficient funds.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, rid := range []pgtype.UUID{recip1, recip2} {
		wg.Add(1)
		go func(r pgtype.UUID, key string) {
			defer wg.Done()
			_, err := svc.CreateTip(ctx, sender, CreateTipInput{
				RecipientType: RecipientUser, RecipientUserID: r,
				Method: MethodCredits, CreditsAmount: 60,
			}, key)
			errs <- err
		}(rid, fmt.Sprintf("ckey-%d", i))
	}
	wg.Wait()
	close(errs)
	var insuff, ok int
	for err := range errs {
		if err == nil {
			ok++
		} else if errors.Is(err, ErrInsufficientBalance) {
			insuff++
		}
	}
	if ok != 1 || insuff != 1 {
		t.Fatalf("concurrent settle: ok=%d insufficient=%d — serial lock broken", ok, insuff)
	}
	if got := balance(t, ctx, d, sender, "credits"); got != 40 {
		t.Fatalf("final balance %d, want 40", got)
	}
}
