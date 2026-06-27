# Phase 5 Implementation Patterns

**Date:** 2026-06-27
**Phase:** 5
**Purpose:** Implementation patterns learned during Phase 5 (Sections 25, 17/28 cross-cut), reusable in future sections.

## Pattern: Inline Struct Types with Raw pgx Queries

The advertising store (`internal/advertising/store.go`) uses raw SQL queries with
hand-written inline struct types instead of sqlc-generated queries. This was
necessary because the store was implemented before `sqlc generate` produced the
advertising query types.

```go
type AdSlotType struct {
    ID                 pgtype.UUID        `json:"id"`
    TenantID           pgtype.UUID        `json:"tenant_id"`
    SlotName           string             `json:"slot_name"`
    Description        *string            `json:"description,omitempty"`
    MaxDisplayPerPage  int32              `json:"max_display_per_page"`
    IsActive           bool               `json:"is_active"`
    CreatedAt          pgtype.Timestamptz `json:"created_at"`
}

func (s *Store) listAdSlotTypes(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]AdSlotType, error) {
    rows, err := s.write.Reader.Query(ctx,
        `SELECT id, tenant_id, slot_name, description, max_display_per_page, is_active, created_at
         FROM ad_slot_types WHERE tenant_id = $1 ORDER BY slot_name LIMIT $2 OFFSET $3`,
        tenantID, limit, offset)
    if err != nil {
        return nil, fmt.Errorf("listing ad slot types: %w", err)
    }
    defer rows.Close()

    var results []AdSlotType
    for rows.Next() {
        var item AdSlotType
        if err := rows.Scan(&item.ID, &item.TenantID, &item.SlotName, &item.Description,
            &item.MaxDisplayPerPage, &item.IsActive, &item.CreatedAt); err != nil {
            return nil, fmt.Errorf("scanning ad slot type: %w", err)
        }
        results = append(results, item)
    }
    return results, rows.Err()
}
```

The Store struct uses `write *db.DB` (not the standard `read/write *gen.Queries`
split) because raw pgxpool connections don't need sqlc's pool split:

```go
type Store struct {
    write *db.DB
}

func NewStore(database *db.DB) *Store {
    return &Store{write: database}
}
```

**Trade-off:** Raw SQL gives flexibility during initial implementation but lacks
the type safety of sqlc-generated queries. Manual scanning is error-prone
(mismatched column order, wrong types). Once `sqlc generate` produces
`gen/advertising.sql.go`, the store should be migrated to use generated types.
Until then, inline struct types and manual scanning are the working pattern.

## Pattern: Credit Bypasses Provider Abstraction

Credit payments (Section 28 cross-cut with Section 17) do NOT implement the
`Provider` interface. There is no external vendor, no `Send()` call, no
webhook. Instead, `PayWithCredit` is a direct synchronous method on the
payments `Service` that debits the wallet balance and creates a payment
record in the same flow:

```go
type CreditSpender interface {
    SpendCredits(ctx context.Context, tenantID pgtype.UUID, userID pgtype.UUID, input CreditSpendInput) (int64, error)
}

type CreditSpendInput struct {
    Amount      int64
    Source      string
    Reference   string
    PurposeType string
    PurposeID   string
}

func (s *Service) PayWithCredit(ctx context.Context, tenantID, userID pgtype.UUID, amount int64, purposeType, purposeID string) (int64, error) {
    if s.wallet == nil {
        return 0, fmt.Errorf("wallet service not configured")
    }
    if amount <= 0 {
        return 0, fmt.Errorf("amount must be positive")
    }
    if purposeType == "" {
        return 0, fmt.Errorf("purpose_type is required")
    }

    input := CreditSpendInput{
        Amount:      amount,
        Source:      "payment",
        Reference:   fmt.Sprintf("payment:%s:%s", purposeType, purposeID),
        PurposeType: purposeType,
        PurposeID:   purposeID,
    }
    newBalance, err := s.wallet.SpendCredits(ctx, tenantID, userID, input)
    if err != nil {
        return 0, fmt.Errorf("%w: %w", ErrInsufficientCredit, err)
    }

    _, err = s.CreatePayment(ctx, tenantID, userID, CreatePaymentInput{
        Provider:    ProviderCredit,
        PurposeType: purposeType,
        PurposeID:   purposeID,
        Amount:      amount,
        Currency:    "NGN",
    })
    if err != nil {
        return 0, fmt.Errorf("recording credit payment: %w", err)
    }
    return newBalance, nil
}
```

The `ProviderCredit` constant (`"credit"`) distinguishes credit payments from
external gateway payments in the `payments` table. This is not a no-op
`CreditProvider` forced into the `Provider` interface — it's a deliberate design
choice documented in the AGENTS.md canonical code pattern.

**Trade-off:** `SpendCredits` and `CreatePayment` are two separate calls, not
wrapped in a single database transaction. If `CreatePayment` fails after
`SpendCredits` succeeds, the wallet is debited but no payment record exists.
This is deferred to Phase 6 for transaction wrapping. In practice, the
`CreatePayment` call fails only on system-level errors (DB connection loss), so
the risk of orphaned debits is low.

## Pattern: Empty-Slice Rendering (Never Placeholder)

When no eligible campaigns match an ad slot, `RenderSlot` returns an empty
slice — never `nil`, never a placeholder object. The frontend is responsible
for showing nothing when the slice is empty.

```go
func (s *Service) RenderSlot(ctx context.Context, tenantID, userID pgtype.UUID, slotName string) ([]AdCampaign, error) {
    slotType, err := s.store.getAdSlotTypeByName(ctx, tenantID, slotName)
    if err != nil {
        return []AdCampaign{}, fmt.Errorf("resolving slot type: %w", err)
    }

    campaigns, err := s.store.listActiveCampaignsBySlot(ctx, tenantID, slotType.ID,
        int32(slotType.MaxDisplayPerPage), 0)
    if len(campaigns) == 0 {
        return []AdCampaign{}, nil
    }

    // Apply frequency capping, budget checks, etc.
    var filtered []AdCampaign
    for _, c := range campaigns {
        if c.FrequencyCapPerUser != nil {
            count, _ := s.store.countImpressionsForUserToday(ctx, userID, c.ID)
            if count >= *c.FrequencyCapPerUser {
                continue
            }
        }
        filtered = append(filtered, c)
    }
    if len(filtered) == 0 {
        return []AdCampaign{}, nil
    }
    return filtered, nil
}
```

Note the explicit `return []AdCampaign{}, nil` for every empty case, not
`return nil, nil`. This guarantees the JSON response is `{"data": []}` rather
than `{"data": null}` or a hardcoded placeholder ad.

**Trade-off:** Returning an empty slice is unambiguous and lets the frontend
handle the "no ads" state with whatever treatment is appropriate for the slot
(hide the container, show zero-height placeholder, etc.). A server-side
placeholder would couple the backend to a specific frontend treatment.

## Pattern: Frequency Capping with Partial Index

Ad campaigns support an optional `frequency_cap_per_user` (maximum impressions
per user per day). The check uses a partial index for efficient counting:

```sql
CREATE INDEX idx_ad_events_user_campaign_impression
    ON ad_events(user_id, campaign_id, occurred_at)
    WHERE event_type = 'impression';
```

The Go query counts only today's impressions:

```go
func (s *Store) countImpressionsForUserToday(ctx context.Context, userID, campaignID pgtype.UUID) (int32, error) {
    var count int32
    err := s.write.Reader.QueryRow(ctx,
        `SELECT COUNT(*)::int FROM ad_events
         WHERE user_id = $1 AND campaign_id = $2
           AND event_type = 'impression'
           AND occurred_at >= current_date`, userID, campaignID).Scan(&count)
    return count, err
}
```

**Trade-off:** The partial index ensures that only impression rows (not clicks,
not other event types) are indexed for this lookup, reducing index size by
excluding irrelevant rows. The `current_date` filter avoids a date parameter,
keeping the query simple — this won't work for "last 24 hours" semantics,
only for calendar-day capping. If rolling-window capping is needed later,
replace `current_date` with a parameterized timestamp.

## Pattern: Interface for Cross-Package Testability

The `CreditSpender` interface (defined in the payments package) allows
`wallet.Service` to be mocked in payment tests without a real database:

```go
// In payments/service_test.go:
type mockCreditSpender struct {
    spendFunc func(ctx context.Context, tenantID, userID pgtype.UUID, input CreditSpendInput) (int64, error)
}

func (m *mockCreditSpender) SpendCredits(ctx context.Context, tenantID, userID pgtype.UUID, input CreditSpendInput) (int64, error) {
    return m.spendFunc(ctx, tenantID, userID, input)
}

func TestPayWithCredit_Success(t *testing.T) {
    var capturedInput CreditSpendInput
    svc := &Service{
        wallet: &mockCreditSpender{
            spendFunc: func(_ context.Context, _ pgtype.UUID, _ pgtype.UUID, input CreditSpendInput) (int64, error) {
                capturedInput = input
                return 900, nil
            },
        },
    }
    balance, err := svc.PayWithCredit(context.Background(),
        pgtype.UUID{}, pgtype.UUID{}, 100, "course", "course-1")

    assert.NoError(t, err)
    assert.Equal(t, int64(900), balance)
    assert.Equal(t, int64(100), capturedInput.Amount)
    assert.Equal(t, "course", capturedInput.PurposeType)
}

func TestPayWithCredit_InsufficientBalance(t *testing.T) {
    svc := &Service{
        wallet: &mockCreditSpender{
            spendFunc: func(_ context.Context, _ pgtype.UUID, _ pgtype.UUID, _ CreditSpendInput) (int64, error) {
                return 0, errors.New("insufficient credits")
            },
        },
    }
    _, err := svc.PayWithCredit(context.Background(),
        pgtype.UUID{}, pgtype.UUID{}, 999, "course", "course-1")

    assert.Error(t, err)
    assert.True(t, errors.Is(err, ErrInsufficientCredit))
}
```

This is the canonical pattern for testing cross-package dependencies: define an
interface in the consumer package, implement a mock in the test file, inject via
the `Service` struct. Compare with Phase 4 where every store used sqlc-generated
types that could be tested with a test database directly — cross-package
dependencies require this additional abstraction.

## Pattern: Adapter Wrapping at Server Init

When `wallet.Service` satisfies the `CreditSpender` interface indirectly (via a
`SpendCredits` method with different parameter types), an adapter is used at
the server init level to bridge the two type spaces:

```go
// In server.go:
type walletCreditAdapter struct {
    wallet *wallet.Service
}

func (a *walletCreditAdapter) SpendCredits(ctx context.Context, tenantID, userID pgtype.UUID, input payments.CreditSpendInput) (int64, error) {
    return a.wallet.SpendCredits(ctx, tenantID, userID, wallet.CreditSpendInput{
        Amount:      input.Amount,
        Source:      input.Source,
        Reference:   input.Reference,
        PurposeType: input.PurposeType,
        PurposeID:   input.PurposeID,
    })
}

// Usage at composition root:
walletSvc := wallet.NewService(database)
walletAdapter := &walletCreditAdapter{wallet: walletSvc}
paymentSvc := payments.NewService(database, walletAdapter)
```

**Trade-off:** The adapter keeps both packages clean — each defines its own
input struct without importing the other. The cost is a thin adapter and
field-by-field copying. If the two structs converge over time, the adapter can
be eliminated. If they diverge (one adds fields the other doesn't need), the
adapter pattern isolates the divergence to the composition root rather than
forcing either package to import the other.

## Pattern: Status Enum as String Constants with Transition Validation

The advertising module uses string constants for campaign statuses:

```go
const (
    CampaignStatusDraft    = "draft"
    CampaignStatusActive   = "active"
    CampaignStatusPaused   = "paused"
    CampaignStatusEnded    = "ended"
    CampaignStatusRejected = "rejected"
)
```

The `ActivateCampaign` method validates transitions at the service layer with
inline checks rather than a map-based validator:

```go
func (s *Service) ActivateCampaign(ctx context.Context, id, tenantID pgtype.UUID) (AdCampaign, error) {
    campaign, err := s.store.getAdCampaignByID(ctx, id, tenantID)
    if err != nil {
        return AdCampaign{}, fmt.Errorf("fetching campaign: %w", err)
    }
    if campaign.Status != CampaignStatusDraft && campaign.Status != CampaignStatusPaused {
        return AdCampaign{}, fmt.Errorf("campaign cannot be activated from status: %s", campaign.Status)
    }
    campaign, err = s.store.updateAdCampaignStatus(ctx, id, tenantID, CampaignStatusActive)
    if err != nil {
        return AdCampaign{}, fmt.Errorf("activating campaign: %w", err)
    }
    return campaign, nil
}
```

**Trade-off:** Compare with the Phase 4 status workflow validation pattern that
uses a `map[string][]string` transition map. For simple state machines
(draft → active → paused → ended), inline checks are sufficient and more
readable than a transition map. For complex workflows with many transitions
(e.g., blog review: pending → approved/rejected/changes_requested), prefer the
Phase 4 transition map pattern. The advertising module's simplicity justified
inline validation.

## Lesson: Store Design Preceding sqlc Generate

The advertising store was written using raw pgx queries because `sqlc generate`
had not yet produced the advertising query types. The store struct uses
`write *db.DB` instead of the standard `read/write *gen.Queries` split.

The flow when adding a new database-backed section should be:

1. Write the migration and run it
2. Run `sqlc generate` to produce query types
3. Write the store using `*gen.Queries`

Step 3 was skipped for advertising because `sqlc generate` wasn't run between
steps 1 and 2. The result is functional but diverges from the canonical pattern
documented in Phase 4. Once `sqlc generate` produces `gen/advertising.sql.go`,
the store should be migrated to `*gen.Queries` for type safety and consistency
with every other section.

## Lesson: Sentry Error Pattern

`ErrInsufficientCredit` uses the standard Go sentinel error pattern:

```go
var ErrInsufficientCredit = fmt.Errorf("insufficient credits balance")
```

Callers use `errors.Is()` to match:

```go
balance, err := paymentSvc.PayWithCredit(ctx, tenantID, userID, amount, purposeType, purposeID)
if errors.Is(err, ErrInsufficientCredit) {
    // Return 402 Payment Required with a user-facing message
    return nil, apperr.New(http.StatusPaymentRequired, "Insufficient credits. Please top up your wallet.")
}
if err != nil {
    return nil, fmt.Errorf("processing payment: %w", err)
}
```

This is the canonical Go pattern for domain errors that require programmatic
inspection by callers. Use sentinel errors for domain errors that the caller
needs to branch on; use `fmt.Errorf` with `%w` for contextual wrapping that
adds information without changing the error identity. A sentinel error created
with `fmt.Errorf` (not `errors.New`) still works with `errors.Is` because
`fmt.Errorf` produces a comparable error value.