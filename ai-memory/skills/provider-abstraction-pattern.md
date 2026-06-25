# Skill: Provider Abstraction Pattern (Payments, Email, AI)

This shape repeats three times in this codebase (Payment Core, Email, AI Gateway) — use it consistently rather than inventing a fourth variant.

## Interface shape
```go
type Provider interface {
    Send(ctx context.Context, req Request) (Response, error)
    VerifyWebhookSignature(payload []byte, sig string) error // where applicable
}
```

## Circuit breaker fallback
Each domain (payments, email, AI) has a priority-ordered list of configured providers per category/task. On failure (timeout or error), the dispatcher retries the next provider in order rather than failing the whole operation, and logs the failure for admin visibility (cost/health dashboards in Control Panel).

## Adding a new provider
1. Implement the interface.
2. Register it with a feature flag / admin-configurable entry (never hardcode availability).
3. No changes needed to the table schema storing `provider` as a string, or to business logic calling the interface.

## What this pattern does NOT need
A new provider should never require touching code outside its own implementation file plus the registration entry. If it does, the abstraction has leaked — fix the interface, don't special-case the caller.
