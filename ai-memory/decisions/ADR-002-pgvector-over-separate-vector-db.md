# ADR-002: pgvector Over a Separate Vector Database

**Status:** Accepted

**Context:** AI Core needs embedding storage for context/retrieval (Honcho integration, AI Gateway context engine).

**Decision:** pgvector extension on the existing PostgreSQL 18.x instance, not a dedicated vector database (Qdrant, Pinecone, etc.).

**Rationale:** Avoids operating a second database system for a workload that doesn't yet justify the operational overhead. Keeps embeddings transactionally consistent with the relational data they're derived from.

**Consequences:** Revisit if embedding query volume or dimensionality grows past what pgvector handles comfortably on the existing instance — not expected at v1/v2 scale.

**Addendum — distinct from Honcho's own database (post-audit):** This decision rejects a *separate vector database system* (Qdrant, Pinecone) for our own embedding storage. It's a different question from where Honcho's own application data lives — Honcho is self-hosted software with its own schema, and runs as a separate logical database (`honcho`) on the same Postgres 18.x instance (Section 01), not inside our application's database. Same instance, two databases — that's logical separation, not "a second database system" in the sense this ADR was avoiding.
