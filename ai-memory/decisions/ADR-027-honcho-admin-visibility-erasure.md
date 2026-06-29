# ADR-027: Honcho Admin Visibility and Erasure Integration

**Status:** Accepted

**Context:** Honcho (Section 20) stores persistent personal AI context outside the main application's tables, in its own database. Two gaps existed once `ai.context.*` permission gating (ADR-025) was in place: admin had no way to see whether Honcho itself was healthy, no way to see what it had actually stored for a given user, and the existing erasure mechanism (ADR-014) — built around same-row PII anonymization — had no path to reach data that lives in a different database entirely.

**Decision:**
- **Control Panel** (Section 21): Honcho connectivity, sync lag, and store size join the existing infrastructure health groups, same pattern as Meilisearch health.
- **Admin Panel** (Section 22): a per-user "AI Context" panel in the existing user management area, showing what Honcho has stored/derived for that user. Read-only, plus the ability to delete an individual entry — no editing. Gated by two new permission nodes, `ai.context.admin_view` and `ai.context.admin_delete_entry`, distinct from the `ai.context.*` category nodes (ADR-025) that gate what feeds the AI in the first place — one family is "what can the AI know," the other is "what can an admin inspect."
- **Erasure** (Section 19, ADR-014): the erasure job gains a step that calls Honcho's deletion API for the erased user's ID, alongside the existing same-row PII wipe.

**Rationale:**
- Health monitoring follows the exact pattern already established for every other external/self-hosted dependency (Meilisearch, SeaweedFS) — no new pattern needed, just a new entry in an existing list.
- Per-user visibility serves two concrete needs: support ("why did JapaBot say that to me") and trust & safety review, both already-established Admin Panel responsibilities for other systems (moderation queue, appeals).
- No-edit, delete-only avoids a subtler problem than it looks: letting an admin hand-edit what an AI system "believes" about a user is a different and riskier capability than letting them delete and let it re-derive naturally from future interaction. Delete-only keeps the intervention simple and auditable.
- Honcho's separate database (Section 01, ADR-002 addendum) means erasure cannot reach it via the existing row-anonymization SQL — it structurally requires a separate deletion call, the same way any other external service integration would.

**Consequences:**
- Two new permission nodes need registering in the RBAC admin UI alongside the `ai.context.*` category family, with documentation distinguishing the two families' purposes so they aren't confused.
- The erasure job (Section 19) is now a two-step process — same-row anonymization plus an external Honcho deletion call — and should be treated as a single logical unit that must both succeed, not a "best effort" Honcho call that's allowed to silently fail while the rest of erasure proceeds.
- Control Panel's health-group pattern now has a precedent for adding future self-hosted dependencies without a new ADR each time — the convention itself, established here and by the Meilisearch precedent, is the reusable artifact.
