# ADR-014: Soft-Delete-and-Anonymize for Account Erasure

**Status:** Accepted

**Context:** GDPR/NDPR right-to-erasure needed a concrete mechanism. A hard delete of the user row would cascade through dozens of foreign-key relationships (posts, comments, reviews, DMs, payments, audit logs, claims, memberships) or require nullable-FK-plus-placeholder logic duplicated across the entire codebase.

**Decision:** The user row is never hard-deleted. Erasure flips status to `deleted` and wipes PII columns in place (email, name, phone, avatar, bio, precise location) on the same row, preserving the row's ID so every foreign-key reference across the platform stays valid without special-case handling. Authored content is anonymized (shown as "Deleted User") rather than removed.

**Rationale:** This was a deliberate choice between two real options — anonymize-and-keep vs. hard-delete-everything — decided explicitly rather than defaulted into, because it affects community continuity (forum threads with a deleted participant's replies still intact vs. orphaned gaps).

**Consequences:** Avatar files are physically deleted from object storage (no reuse case to worry about). Payment records keep their data per the legal retention exception, already disassociated from PII via the same row-level anonymization.

**Addendum (post-audit):** The erasure job now also calls Honcho's deletion API for the user's ID, alongside the same-row PII wipe — see Section 19 amendment and ADR-027. Honcho runs as its own service with its own database; it has no foreign key into `users`, so its data isn't touched by the row-anonymization mechanism above and needs an explicit deletion call as a separate step in the same job.
