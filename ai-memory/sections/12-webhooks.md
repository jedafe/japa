# Section 12 — Webhooks (Outbound)

**Status:** ✅ Locked

## Decided
Signed payloads (HMAC-SHA256, per-subscription secret), retry via the `webhook_delivery` critical-tier task, delivery log. Versioned envelope: `{ event_id, event_type, occurred_at, schema_version, data }`. Event catalog scoped to non-PII business events: `content.published`, `content.updated`, `content.deleted`, `claim.approved`, `claim.rejected`, `review.created`, `organization.verified`, `payment.succeeded`, `payment.failed`. Challenge/response endpoint verification before a subscription activates — closes the SSRF-probe risk and confirms registrant ownership. Admin-only management in v1; org self-service explicitly deferred to v2.
