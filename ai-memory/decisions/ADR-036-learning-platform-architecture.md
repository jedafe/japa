# ADR-036: Learning Platform Architecture

**Status:** Accepted

**Context:** JapaConnect needed a learning platform at learn.japaconnect.com. The question was standalone app vs integrated into the main site.

**Decision:** Separate SvelteKit SSR app (fourth app, shared Go API backend). Dual-access: Pro subscription = all tracks, course_access payment = individual track. Schema: learning_tracks → learning_modules → learning_lessons (article/video/quiz/checklist). TipTap JSON for lesson content. PDF certificates via learning:issue_certificate Asynq task, stored in MinIO.

**Rationale:** Separate subdomain keeps main site focused; shared API avoids data duplication. PDF generation is CPU-intensive — Asynq delegation avoids blocking the API.

**Consequences:** Fifth SvelteKit app adds build/deploy surface. Draft tracks excluded from discovery until published.
