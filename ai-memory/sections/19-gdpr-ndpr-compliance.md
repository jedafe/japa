# Section 19 — GDPR / NDPR Compliance

**Status:** ✅ Locked

## Decided
Consent records with policy versioning (`consent_records`: user_id nullable, consent_type, granted, policy_version, ip_address, timestamp). Cookie banner scope reduced in practice since Umami is cookieless by default — mainly relevant once ad-tracking pixels (Section 25) exist. Data export via `data_export` task, delivered as a signed time-limited download link (24-48h), never rendered inline indefinitely.

Right to erasure: soft-delete-and-anonymize on the same user row, never a hard delete (see ADR-014) — preserves FK integrity across the entire platform. Authored content keeps its body, author shows as "Deleted User." Avatar files physically deleted; tokens revoked. Payment records retained per the legal retention exception built into the erasure right itself, PII link already severed via the same row anonymization.

Explicitly flagged as non-engineering, needing legal/operational attention rather than architecture: actual privacy policy text, DPO designation, breach notification procedure.

## See Also
ADR-014.

## Amendment — Honcho Erasure Integration (post-audit)

Honcho (Section 20) stores persistent personal context outside the main `users` row. The erasure job now includes a step that calls Honcho's own deletion API for the erased user's ID, in addition to the existing same-row PII wipe — Honcho's store isn't reachable via SQL against the main row, so the existing mechanism alone would leave personal data behind in AI memory after an otherwise-complete erasure. See ADR-014 addendum and ADR-027.
