# Skill: Wallet Ledger Pattern

## Immutable log, never a balance column

Points and Credits both work the same way: every change is an INSERT, never an UPDATE to a stored balance. Current balance is the `resulting_balance` of the latest row for that user+currency.

```sql
-- name: InsertLedgerEntry :one
INSERT INTO wallet_ledger (tenant_id, user_id, currency, type, amount, resulting_balance, source, actor_id, reference, created_at)
VALUES (@tenant_id, @user_id, @currency, @type, @amount, @resulting_balance, @source, @actor_id, @reference, now())
RETURNING id;
```

`currency` is `points` or `credits`. `type` covers `earn`, `convert_out`, `convert_in`, `purchase`, `spend`, `reversal`, `admin_adjustment`. Read current balance by querying the latest row per `(user_id, currency)`, not a separate counter — the log is the only source of truth.

## Activity Point Rules — event-driven, not manual

Don't write points directly from feature code. Emit a typed event referencing an `activity_key`; a consumer resolves the rule and writes the ledger entry.

```go
// In the feature module (e.g. Forum, after a post is created)
events.Emit(ctx, ActivityEvent{
    ActivityKey: "forum_post_created",
    UserID:      post.AuthorID,
    Reference:   "forum_post:" + post.ID.String(), // idempotency key
})
```

```go
// Points engine consumer (Asynq, default tier)
func ProcessActivityEvent(ctx context.Context, ev ActivityEvent) error {
    if alreadyProcessed(ctx, ev.Reference) { // idempotency check
        return nil
    }
    rule, ok := activityRules.Lookup(ev.ActivityKey)
    if !ok || !rule.Enabled {
        return nil
    }
    if rule.Cap != nil && capExceeded(ctx, ev.UserID, ev.ActivityKey, rule.Cap) {
        return nil
    }
    return ledger.Insert(ctx, ev.UserID, "points", rule.PointValue, ev.Reference)
}
```

Rule value changes apply forward-only — never rescore historical ledger entries when an admin edits a rule.

## Credit spend bypasses the Provider interface

Unlike Paystack/Stripe/NOWPayments, Credit has no external party — don't implement a `Provider` for it. Debit the ledger synchronously, in the same transaction as the `payments` row, at checkout time:

```go
func PayWithCredit(ctx context.Context, tx pgx.Tx, userID uuid.UUID, amount int64, purposeType, purposeID string) error {
    balance, err := getCurrentBalance(ctx, tx, userID, "credits")
    if err != nil || balance < amount {
        return ErrInsufficientCredit
    }
    if err := insertLedgerEntry(ctx, tx, userID, "credits", "spend", -amount, balance-amount, purposeType, purposeID); err != nil {
        return err
    }
    return insertPaymentRow(ctx, tx, userID, "credit", purposeType, purposeID, amount, "succeeded")
}
```

No webhook, no `VerifyWebhookSignature` stub — that would be forcing a vendor-abstraction shape onto something that isn't a vendor.

## Reversals can go negative

A clawback (fraudulent referral, refunded course) is a new ledger row with a negative `amount`, same as any other entry. Don't block a reversal because it would take the balance below zero — let it go negative and leave collection policy to manual admin review.
