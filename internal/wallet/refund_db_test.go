package wallet

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"errors"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
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

func newRefundUser(t *testing.T, ctx context.Context, d *db.DB) pgtype.UUID {
	t.Helper()
	uname := fmt.Sprintf("wallettest_%d", time.Now().UnixNano())
	var id pgtype.UUID
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email) VALUES ($1, $2) RETURNING id`,
		uname, uname+"@test.dev").Scan(&id); err != nil {
		t.Fatalf("inserting user: %v", err)
	}
	return id
}

func seedCreditEntry(t *testing.T, ctx context.Context, d *db.DB, userID pgtype.UUID, amount, balance int64) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	src := "refund_test"
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO credits_ledger (user_id, operation, amount, resulting_balance, source)
		 VALUES ($1, 'earn', $2, $3, $4) RETURNING id`,
		userID, amount, balance, src).Scan(&id); err != nil {
		t.Fatalf("seeding credits entry: %v", err)
	}
	return id
}

func seedPointsEntry(t *testing.T, ctx context.Context, d *db.DB, userID pgtype.UUID, amount, balance int64) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	src := "refund_test"
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO wallet_ledger (user_id, ledger_type, operation, amount, resulting_balance, source)
		 VALUES ($1, 'points', 'earn', $2, $3, $4) RETURNING id`,
		userID, amount, balance, src).Scan(&id); err != nil {
		t.Fatalf("seeding points entry: %v", err)
	}
	return id
}

func creditBalance(t *testing.T, ctx context.Context, d *db.DB, userID pgtype.UUID) int64 {
	t.Helper()
	var bal *int64
	if err := d.Reader.QueryRow(ctx,
		`SELECT resulting_balance FROM credits_ledger WHERE user_id=$1 ORDER BY seq DESC LIMIT 1`,
		userID).Scan(&bal); err != nil || bal == nil {
		return 0
	}
	return *bal
}

func TestDBCreditRefundFullAndPartial(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := NewService(d)
	user := newRefundUser(t, ctx, d)
	entry := seedCreditEntry(t, ctx, d, user, 100, 100)

	// Partial refund of 30 — debits the user, links reverses_id.
	bal, err := svc.AdminRefundCreditEntry(ctx, user, entry, AdminReverseInput{
		Amount: ptrI64(30), Reason: "partial one", RefundID: "r1",
	})
	if err != nil {
		t.Fatalf("partial refund: %v", err)
	}
	if bal != 70 {
		t.Fatalf("balance after partial = %d, want 70", bal)
	}

	// Same refund_id is a retry — ErrDuplicate, not a second debit.
	if _, err := svc.AdminRefundCreditEntry(ctx, user, entry, AdminReverseInput{
		Amount: ptrI64(30), Reason: "retry", RefundID: "r1",
	}); !errors.Is(err, errpkg.ErrDuplicate) {
		t.Fatalf("retry refund_id: got %v, want ErrDuplicate", err)
	}

	// Over-refund exceeds the unrefunded remainder.
	if _, err := svc.AdminRefundCreditEntry(ctx, user, entry, AdminReverseInput{
		Amount: ptrI64(71), Reason: "too much", RefundID: "r2",
	}); !errors.Is(err, errpkg.ErrInvalid) {
		t.Fatalf("over-refund: got %v, want ErrInvalid", err)
	}

	// Nil amount refunds the remaining 70.
	bal, err = svc.AdminRefundCreditEntry(ctx, user, entry, AdminReverseInput{
		Reason: "rest", RefundID: "r3",
	})
	if err != nil {
		t.Fatalf("remainder refund: %v", err)
	}
	if bal != 0 {
		t.Fatalf("balance after remainder = %d, want 0", bal)
	}

	// reverses_id linkage landed on the refund rows.
	var linked int
	if err := d.Reader.QueryRow(ctx,
		`SELECT count(*) FROM credits_ledger WHERE reverses_id = $1`, entry).Scan(&linked); err != nil || linked != 2 {
		t.Fatalf("linked refunds = %d (err=%v), want 2", linked, err)
	}
}

func TestDBCreditRefundFloor(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := NewService(d)
	user := newRefundUser(t, ctx, d)
	entry := seedCreditEntry(t, ctx, d, user, 100, 100)

	// Spend 60 so only 40 remains — refunding the full 100 must fail the floor.
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO credits_ledger (user_id, operation, amount, resulting_balance, source)
		 VALUES ($1, 'spend', -60, 40, 'refund_test')`, user); err != nil {
		t.Fatalf("seeding spend: %v", err)
	}

	if _, err := svc.AdminRefundCreditEntry(ctx, user, entry, AdminReverseInput{Reason: "full"}); !errors.Is(err, errpkg.ErrInsufficientBalance) {
		t.Fatalf("floor: got %v, want ErrInsufficientBalance", err)
	}
}

func TestDBPointsReversalStillWorks(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := NewService(d)
	user := newRefundUser(t, ctx, d)
	entry := seedPointsEntry(t, ctx, d, user, -50, -50)

	// Undoing a penalty on a negative points balance is an upward move.
	bal, err := svc.AdminReverseLedgerEntry(ctx, user, entry, AdminReverseInput{Reason: "undo"})
	if err != nil {
		t.Fatalf("points reversal: %v", err)
	}
	if bal != 0 {
		t.Fatalf("points balance after reversal = %d, want 0", bal)
	}

	// Points stay full-reversal only.
	entry2 := seedPointsEntry(t, ctx, d, user, 10, 10)
	if _, err := svc.AdminReverseLedgerEntry(ctx, user, entry2, AdminReverseInput{
		Amount: ptrI64(5), Reason: "partial",
	}); !errors.Is(err, errpkg.ErrInvalid) {
		t.Fatalf("partial points: got %v, want ErrInvalid", err)
	}
}

func ptrI64(v int64) *int64 { return &v }

var _ = gen.WalletLedger{} // keep gen import for param-shape drift checks
