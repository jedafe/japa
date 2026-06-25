# ADR-001: Go Monolith Over FastAPI/Laravel/NestJS

**Status:** Accepted

**Context:** Needed a backend stack that handles WebSockets, background job processing (Asynq), and a complex relational data layer well, for a solo developer to build and operate alone.

**Decision:** Go monolith (Chi router, sqlc, pgx/v5, Asynq), validated against FastAPI, Laravel, and NestJS.

**Rationale:** Go's concurrency model fits WebSockets and Asynq naturally without bolting on extra infrastructure. Single binary deployment simplifies ops for a solo founder. Memory efficiency matters on a modest VPS. sqlc + pgx/v5 give compile-time-checked SQL without an ORM's abstraction tax for a data layer this complex.

**Consequences:** Slower initial velocity than a framework like Laravel for pure CRUD scaffolding, offset by stronger guarantees at the data layer and no separate runtime needed for background jobs.
