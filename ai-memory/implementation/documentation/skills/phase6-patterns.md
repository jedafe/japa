# Phase 6 Implementation Patterns

**Date:** 2026-06-27
**Phase:** 6
**Purpose:** Implementation patterns learned during Phase 6 (Sections 11, 19, 21, 22, 38, 40, 41, 42, 43), reusable in future sections.

## Pattern: Asynq Three-Tier Queue Architecture

The job queue uses three priority tiers with distinct concurrency limits:

```go
type Config struct {
    CriticalConcurrency int // default: 10
    DefaultConcurrency  int // default: 5
    LowConcurrency      int // default: 2
    RedisAddr           string // JC_REDIS_ADDR
    RedisPassword       string // JC_REDIS_PASSWORD
    DBDrainTimeout      time.Duration
}

func NewConfig() Config {
    return Config{
        CriticalConcurrency: getEnvInt("JC_WORKER_CRITICAL_CONCURRENCY", 10),
        DefaultConcurrency:  getEnvInt("JC_WORKER_DEFAULT_CONCURRENCY", 5),
        LowConcurrency:      getEnvInt("JC_WORKER_LOW_CONCURRENCY", 2),
        RedisAddr:           getEnv("JC_REDIS_ADDR", "localhost:6379"),
        RedisPassword:       getEnv("JC_REDIS_PASSWORD", ""),
        DBDrainTimeout:      time.Duration(getEnvInt("JC_WORKER_DRAIN_TIMEOUT_SECS", 30)) * time.Second,
    }
}
```

Task types are string constants organized by tier:

```go
const (
    // Critical tier (concurrency 10)
    TypePaymentWebhookProcess = "payment_webhook_process"
    TypeNotificationDispatch  = "notification_dispatch"
    TypeWebhookDelivery       = "webhook_delivery"
    TypeEmailSend             = "email_send"
    TypeTicketConfirm         = "ticket_confirm"

    // Default tier (concurrency 5)
    TypeSearchIndexSync    = "search_index_sync"
    TypeImageProcess       = "image_process"
    TypeAITaskProcess      = "ai_task_process"
    TypeEngagementUpdate   = "engagement_update"
    TypeModerationScan     = "moderation_scan"

    // Low tier (concurrency 2)
    TypeScheduledPublish   = "scheduled_publish"
    TypeLinkPreview        = "link_preview"
    TypeMediaScan          = "media_scan"
    TypeAnalyticsRefresh   = "analytics_refresh"
    TypeDBVacuum           = "db_vacuum"
)
```

The `RegisterHandlers` function maps task types to handler functions:

```go
func RegisterHandlers(mux *asynq.ServeMux) {
    mux.HandleFunc(TypePaymentWebhookProcess, handlePaymentWebhookProcess)
    mux.HandleFunc(TypeNotificationDispatch, handleNotificationDispatch)
    // ... 38 total handlers
}
```

The worker shares the same Valkey instance as the cache layer (same `JC_REDIS_ADDR`). This is intentional — separate instances would complicate deployment without performance benefit, since queue and cache workloads have different access patterns.

**Trade-off:** Three tiers with fixed concurrency limits are simpler to reason about than dynamic scaling. The downside is that a burst of critical tasks can starve low-priority tasks. In practice, the low tier handles background maintenance (DB vacuum, sitemap regeneration) that can tolerate delays. If starvation becomes an issue, add a minimum concurrency guarantee per tier.

## Pattern: Interface-Based Processor Injection for Asynq Workers

Long-running background tasks (GDPR data export, analytics aggregation) need to call service-layer methods without creating circular imports. The pattern uses interfaces defined at the worker level:

```go
// In internal/worker/tasks.go:
type GDPRProcessor interface {
    ProcessDataExport(ctx context.Context, requestID pgtype.UUID) error
    ProcessAccountDeletion(ctx context.Context, requestID pgtype.UUID) error
}

type AnalyticsProcessor interface {
    RefreshMaterializedViews(ctx context.Context, tenantID pgtype.UUID) error
    ComputeEngagementScores(ctx context.Context, tenantID pgtype.UUID) error
}
```

The server creates the processor implementations and passes them to the worker:

```go
// In internal/server/server.go:
gdprProc := gdpr.NewProcessor(database, mailer)
analyticsProc := analytics.NewProcessor(database)

workerServer := worker.NewServer(workerConfig)
workerServer.RegisterGDPRProcessor(gdprProc)
workerServer.RegisterAnalyticsProcessor(analyticsProc)
```

The worker handlers call through the interface:

```go
func (s *Server) handleProcessDataExport(ctx context.Context, task *asynq.Task) error {
    var payload struct {
        RequestID string `json:"request_id"`
    }
    if err := json.Unmarshal(task.Payload(), &payload); err != nil {
        return fmt.Errorf("unmarshaling payload: %w", err)
    }
    requestID, err := uuid.Parse(payload.RequestID)
    if err != nil {
        return fmt.Errorf("parsing request_id: %w", err)
    }
    return s.gdprProcessor.ProcessDataExport(ctx, requestID)
}
```

**Trade-off:** This adds indirection compared to direct function calls, but the alternative (importing `internal/gdpr` from `internal/worker`) would create a circular dependency since `internal/gdpr` imports `internal/worker` for Asynq task enqueueing. The interface boundary keeps the dependency graph acyclic.

## Pattern: IP Block Middleware with CIDR Containment

The spam/safety system blocks IPs using PostgreSQL's `<<=` inet operator for CIDR containment matching:

```sql
CREATE TABLE ip_blocks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    ip_address inet NOT NULL,
    cidr_range inet NOT NULL,
    block_type VARCHAR(50) NOT NULL, -- 'full', 'registration', 'posting', 'api'
    reason TEXT,
    blocked_by UUID REFERENCES users(id),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ip_blocks_cidr ON ip_blocks USING gist (cidr_range);
```

The middleware runs after `RealIP` (to get the true client IP) and before route handlers:

```go
func IPBlockMiddleware(store *Store) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ip := net.ParseIP(r.Context().Value("real_ip").(string))
            if ip == nil {
                next.ServeHTTP(w, r)
                return
            }

            blocked, err := store.IsIPBlocked(r.Context(), ip, getBlockType(r))
            if err != nil {
                // Log but don't block on DB errors
                slog.ErrorContext(r.Context(), "ip_block_check_failed", "error", err)
                next.ServeHTTP(w, r)
                return
            }
            if blocked {
                apperr.Respond(w, r, apperr.New(http.StatusForbidden, "Access denied"))
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

func (s *Store) IsIPBlocked(ctx context.Context, ip net.IP, blockType string) (bool, error) {
    var blocked bool
    err := s.write.Reader.QueryRow(ctx,
        `SELECT EXISTS(
            SELECT 1 FROM ip_blocks
            WHERE cidr_range >>= $1
              AND (block_type = 'full' OR block_type = $2)
              AND (expires_at IS NULL OR expires_at > NOW())
        )`, ip.String(), blockType).Scan(&blocked)
    return blocked, err
}
```

The `block_type` parameter determines scope: `'full'` blocks all requests, `'registration'` only blocks signups, `'posting'` only blocks content creation, `'api'` only blocks API access.

**Trade-off:** PostgreSQL's GiST index on `cidr_range` handles CIDR containment efficiently, but `>>=` is slower than exact IP matching. For high-traffic endpoints, consider caching blocked IP ranges in Valkey with a short TTL. The middleware already logs errors without blocking, so a cache miss degrades gracefully to the DB query.

## Pattern: Censored Words Valkey Cache with TTL

The spam system checks content against a list of censored words. To avoid querying the database on every content creation, the list is cached in Valkey:

```go
func (s *Store) GetCensoredWords(ctx context.Context, tenantID pgtype.UUID) ([]string, error) {
    cacheKey := fmt.Sprintf("jc:spam:censored:%s", tenantID.String())

    // Check cache first
    cached, err := s.rdb.Get(ctx, cacheKey).Result()
    if err == nil {
        var words []string
        if json.Unmarshal([]byte(cached), &words) == nil {
            return words, nil
        }
    }

    // Fallback to DB
    rows, err := s.write.Reader.Query(ctx,
        `SELECT word FROM censored_words WHERE tenant_id = $1 AND is_active = true`,
        tenantID)
    if err != nil {
        return nil, fmt.Errorf("querying censored words: %w", err)
    }
    defer rows.Close()

    var words []string
    for rows.Next() {
        var word string
        if err := rows.Scan(&word); err != nil {
            return nil, fmt.Errorf("scanning censored word: %w", err)
        }
        words = append(words, word)
    }

    // Cache for 1 hour
    data, _ := json.Marshal(words)
    s.rdb.Set(ctx, cacheKey, string(data), time.Hour)

    return words, rows.Err()
}
```

Cache is invalidated on admin create/delete:

```go
func (s *Store) CreateCensoredWord(ctx context.Context, tenantID pgtype.UUID, word string) error {
    _, err := s.write.Writer.Exec(ctx,
        `INSERT INTO censored_words (tenant_id, word) VALUES ($1, $2)`,
        tenantID, word)
    if err != nil {
        return fmt.Errorf("creating censored word: %w", err)
    }
    // Invalidate cache
    s.rdb.Del(ctx, fmt.Sprintf("jc:spam:censored:%s", tenantID.String()))
    return nil
}
```

**Trade-off:** The 1-hour TTL means newly added words take up to an hour to take effect if the cache isn't manually invalidated. The explicit `Del` on create/delete eliminates this window for admin actions, but bulk imports (CSV upload of 1000 words) would still benefit from explicit cache invalidation after the batch completes.

## Pattern: Ticket Status State Machine

The ticket system enforces valid status transitions at the service layer:

```go
var validTransitions = map[string][]string{
    "open":              {"in_progress", "closed", "spam"},
    "in_progress":       {"waiting_on_user", "resolved", "closed", "spam"},
    "waiting_on_user":   {"in_progress", "resolved", "closed", "spam"},
    "resolved":          {"in_progress", "closed", "spam"},
    "closed":            {"open"}, // reopen
    "spam":              {"open"}, // unspam
}

func (s *Service) TransitionTicket(ctx context.Context, id, tenantID pgtype.UUID, newStatus string, userID pgtype.UUID) (Ticket, error) {
    ticket, err := s.store.GetTicketByID(ctx, id, tenantID)
    if err != nil {
        return Ticket{}, fmt.Errorf("fetching ticket: %w", err)
    }

    allowed := validTransitions[ticket.Status]
    valid := false
    for _, status := range allowed {
        if status == newStatus {
            valid = true
            break
        }
    }
    if !valid {
        return Ticket{}, fmt.Errorf("invalid transition: %s → %s", ticket.Status, newStatus)
    }

    // Transition and log event
    ticket, err = s.store.UpdateTicketStatus(ctx, id, tenantID, newStatus)
    if err != nil {
        return Ticket{}, fmt.Errorf("updating status: %w", err)
    }

    err = s.store.LogTicketEvent(ctx, id, tenantID, TicketEvent{
        EventType: "status_change",
        OldValue:  ticket.Status,
        NewValue:  newStatus,
        ActorID:   userID,
    })
    if err != nil {
        return Ticket{}, fmt.Errorf("logging event: %w", err)
    }

    return ticket, nil
}
```

Admin-only transitions (spam, close from any state) are checked at the handler level via permission keys, not in the state machine itself.

**Trade-off:** The state machine is simple enough for a map-based validator. If transitions become conditional (e.g., "can only close if no open SLA breaches"), extract to a function that inspects the ticket's current state beyond just the status field.

## Pattern: Auto-Assignment via Least-Loaded Query

Tickets in a group are assigned to the agent with the fewest open tickets:

```sql
WITH agent_loads AS (
    SELECT
        u.id AS agent_id,
        COUNT(t.id) FILTER (WHERE t.status IN ('open', 'in_progress', 'waiting_on_user')) AS open_count
    FROM users u
    JOIN user_roles ur ON ur.user_id = u.id
    JOIN roles r ON r.id = ur.role_id
    LEFT JOIN tickets t ON t.assigned_to = u.id AND t.tenant_id = $1
    WHERE r.name = 'support_agent'
      AND u.is_active = true
    GROUP BY u.id
)
SELECT agent_id FROM agent_loads
ORDER BY open_count ASC
LIMIT 1;
```

The query and assignment happen in a single transaction:

```go
func (s *Service) AutoAssignTicket(ctx context.Context, ticketID, tenantID pgtype.UUID) error {
    tx, err := s.write.Writer.Begin(ctx)
    if err != nil {
        return fmt.Errorf("beginning transaction: %w", err)
    }
    defer tx.Rollback(ctx)

    var agentID pgtype.UUID
    err = tx.QueryRow(ctx,
        `WITH agent_loads AS (
            SELECT u.id AS agent_id,
                   COUNT(t.id) FILTER (WHERE t.status IN ('open', 'in_progress', 'waiting_on_user')) AS open_count
            FROM users u
            JOIN user_roles ur ON ur.user_id = u.id
            JOIN roles r ON r.id = ur.role_id
            LEFT JOIN tickets t ON t.assigned_to = u.id AND t.tenant_id = $1
            WHERE r.name = 'support_agent' AND u.is_active = true
            GROUP BY u.id
        ) SELECT agent_id FROM agent_loads ORDER BY open_count ASC LIMIT 1`,
        tenantID).Scan(&agentID)
    if err != nil {
        return fmt.Errorf("finding least-loaded agent: %w", err)
    }

    _, err = tx.Exec(ctx,
        `UPDATE tickets SET assigned_to = $1, status = 'in_progress' WHERE id = $2 AND tenant_id = $3`,
        agentID, ticketID, tenantID)
    if err != nil {
        return fmt.Errorf("assigning ticket: %w", err)
    }

    return tx.Commit(ctx)
}
```

**Trade-off:** The `FILTER` clause (PostgreSQL-specific) counts open tickets per agent efficiently. If supporting other databases, replace with a `CASE WHEN` subquery. The transaction ensures atomicity — no two tickets are assigned to the same "least-loaded" agent simultaneously under low concurrency. Under high concurrency, add `FOR UPDATE` on the agent row to prevent race conditions.

## Pattern: GDPR Account Deletion (Anonymize, Never Delete)

Account deletion anonymizes the user row rather than hard-deleting it:

```go
func (s *Service) ProcessAccountDeletion(ctx context.Context, requestID pgtype.UUID) error {
    request, err := s.store.GetDataExportRequest(ctx, requestID)
    if err != nil {
        return fmt.Errorf("fetching deletion request: %w", err)
    }

    userID := request.UserID
    tenantID := request.TenantID

    // Anonymize user row
    _, err = s.write.Writer.Exec(ctx,
        `UPDATE users SET
            name = 'Deleted User',
            email = NULL,
            phone = NULL,
            bio = NULL,
            avatar_url = NULL,
            is_active = false,
            updated_at = NOW()
        WHERE id = $1 AND tenant_id = $2`,
        userID, tenantID)
    if err != nil {
        return fmt.Errorf("anonymizing user: %w", err)
    }

    // Revoke all refresh tokens
    _, err = s.write.Writer.Exec(ctx,
        `DELETE FROM refresh_tokens WHERE user_id = $1`,
        userID)
    if err != nil {
        return fmt.Errorf("revoking tokens: %w", err)
    }

    // Mark request as completed
    _, err = s.write.Writer.Exec(ctx,
        `UPDATE account_deletion_requests SET status = 'completed', completed_at = NOW() WHERE id = $1`,
        requestID)
    if err != nil {
        return fmt.Errorf("marking request complete: %w", err)
    }

    return nil
}
```

Hard deletion is never used because:
1. Foreign keys reference `users(id)` across dozens of tables (posts, comments, payments, tickets)
2. Audit logs must retain the actor reference even after deletion
3. anonymized data preserves referential integrity without exposing PII

**Trade-off:** Anonymized rows still consume storage. If storage costs become a concern, implement a background job that hard-deletes anonymized users older than 7 years (GDPR retention limit). Until then, the simplicity of "never delete" outweighs the storage cost.

## Pattern: Materialized Views Refreshed CONCURRENTLY

Analytics dashboards read from materialized views that are refreshed periodically:

```sql
CREATE MATERIALIZED VIEW mv_content_engagement AS
SELECT
    c.id AS content_id,
    c.tenant_id,
    COUNT(DISTINCT e.user_id) AS unique_views,
    COUNT(DISTINCT CASE WHEN e.event_type = 'like' THEN e.user_id END) AS likes,
    COUNT(DISTINCT CASE WHEN e.event_type = 'comment' THEN e.user_id END) AS comments,
    MAX(e.occurred_at) AS last_activity_at
FROM contents c
LEFT JOIN engagement_events e ON e.entity_id = c.id AND e.entity_type = 'content'
WHERE c.status = 'published'
GROUP BY c.id, c.tenant_id;

CREATE UNIQUE INDEX idx_mv_content_engagement_id ON mv_content_engagement(content_id);
```

The `CONCURRENTLY` option avoids locking the view during refresh:

```go
func (s *Processor) RefreshMaterializedViews(ctx context.Context, tenantID pgtype.UUID) error {
    views := []string{
        "mv_content_engagement",
        "mv_user_reputation",
        "mv_ticket_metrics",
    }

    for _, view := range views {
        _, err := s.write.Writer.Exec(ctx,
            fmt.Sprintf("REFRESH MATERIALIZED VIEW CONCURRENTLY %s", view))
        if err != nil {
            return fmt.Errorf("refreshing %s: %w", view, err)
        }
    }
    return nil
}
```

Refresh is triggered by an Asynq task (`analytics_refresh_views`) on an hourly cron schedule.

**Trade-off:** `CONCURRENTLY` requires a unique index on the materialized view (otherwise it fails). The trade-off is that reads during refresh see stale data until the refresh completes, but there's no lock contention. For real-time analytics, supplement materialized views with a rolling window query against the raw events table.

## Pattern: Dynamic robots.txt from Bot Registry

The bots module generates `robots.txt` from the `known_bots` database table:

```go
func (s *Service) GenerateRobotsTxt(ctx context.Context, tenantID pgtype.UUID) (string, error) {
    cacheKey := fmt.Sprintf("jc:bots:robots:%s", tenantID.String())

    // Check cache (24h TTL)
    cached, err := s.rdb.Get(ctx, cacheKey).Result()
    if err == nil {
        return cached, nil
    }

    bots, err := s.store.ListKnownBots(ctx, tenantID)
    if err != nil {
        return "", fmt.Errorf("listing bots: %w", err)
    }

    var sb strings.Builder
    sb.WriteString("User-agent: *\nAllow: /\n\n")

    for _, bot := range bots {
        sb.WriteString(fmt.Sprintf("User-agent: %s\n", bot.UserAgent))
        if bot.Blocked {
            sb.WriteString("Disallow: /\n")
        } else if bot.CrawlDelay > 0 {
            sb.WriteString(fmt.Sprintf("Crawl-delay: %d\n", bot.CrawlDelay))
            if bot.AllowedPaths != "" {
                sb.WriteString(fmt.Sprintf("Allow: %s\n", bot.AllowedPaths))
            }
            sb.WriteString("Disallow: /\n")
        }
        sb.WriteString("\n")
    }

    result := sb.String()
    s.rdb.Set(ctx, cacheKey, result, 24*time.Hour)
    return result, nil
}
```

Admin CRUD endpoints manage the bot registry:

```go
func (h *Handlers) HandleCreateBot(w http.ResponseWriter, r *http.Request) {
    var input CreateBotInput
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        apperr.Respond(w, r, apperr.New(http.StatusBadRequest, "Invalid request body"))
        return
    }

    bot, err := h.service.CreateKnownBot(r.Context(), tenantID(r), input)
    if err != nil {
        apperr.Respond(w, r, apperr.Wrap(err))
        return
    }

    // Invalidate robots.txt cache
    h.service.InvalidateRobotsCache(r.Context(), tenantID(r))

    respond.JSON(w, http.StatusCreated, bot)
}
```

**Trade-off:** The 24-hour cache TTL means newly added bots take up to a day to appear in `robots.txt` if the admin doesn't manually trigger a cache invalidation. The explicit invalidation on create/update/delete eliminates this window for admin actions. A background refresh job (Asynq cron) could replace the TTL with a guaranteed freshness guarantee.

## Pattern: SvelteKit SPA with API Client Helper

The Control Panel and Admin Panel are SvelteKit SPAs (`adapter-static`) that communicate with the Go backend via a shared API client module:

```typescript
// src/lib/api.ts
import { browser } from '$app/environment';

const API_BASE = import.meta.env.VITE_API_BASE || 'http://localhost:8080';

interface ApiResponse<T> {
  data: T;
  error?: string;
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown
): Promise<T> {
  const token = browser ? localStorage.getItem('access_token') : null;

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const response = await fetch(`${API_BASE}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  const json: ApiResponse<T> = await response.json();

  if (!response.ok) {
    throw new Error(json.error || `Request failed: ${response.status}`);
  }

  return json.data;
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  delete: <T>(path: string) => request<T>('DELETE', path),
};
```

Components use Svelte 5 runes for reactive state:

```svelte
<script lang="ts">
  import { api } from '$lib/api';

  let tickets = $state<Ticket[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  $effect(() => {
    loadTickets();
  });

  async function loadTickets() {
    try {
      loading = true;
      tickets = await api.get<Ticket[]>('/api/v1/tickets');
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load tickets';
    } finally {
      loading = false;
    }
  }
</script>

{#if loading}
  <p>Loading...</p>
{:else if error}
  <p class="text-red-500">{error}</p>
{:else}
  {#each tickets as ticket (ticket.id)}
    <div>{ticket.subject}</div>
  {/each}
{/if}
```

**Trade-off:** The SPA approach simplifies deployment (static files served by Nginx) but loses SSR benefits (SEO, initial load performance). For internal admin panels, this is acceptable — the Go backend serves the public-facing SSR app. The `browser` guard on `localStorage` prevents SSR errors when SvelteKit pre-renders the page.

## Lesson: Worker Handler Placeholder Pattern

All 38 Asynq handlers start as log-and-return-nil placeholders:

```go
func handlePaymentWebhookProcess(ctx context.Context, task *asynq.Task) error {
    slog.InfoContext(ctx, "payment_webhook_process handler invoked",
        "task_id", task.ID(),
        "payload", string(task.Payload()))
    return nil
}
```

This allows the Asynq infrastructure (server, client, mux, queue configuration) to be built and tested first without waiting for every feature module to be implemented. As each section is built, its handlers are replaced with real implementations.

The flow for wiring a handler:
1. Section builds its service layer with a `Process*` method
2. Worker gets a `Processor` interface matching that method signature
3. Server creates the processor and registers it with the worker
4. Handler is updated to call through the interface instead of logging

This incremental approach prevented Section 11 (Asynq) from blocking all other sections.

## Lesson: Migration Numbering Continuity

Migrations must be numbered sequentially. Phase 6 used numbers 000025 through 000029:

- `000025_worker_infrastructure.sql` — Asynq task tracking
- `000026_ticket_system.sql` — Tickets, comments, attachments, events
- `000027_gdpr_compliance.sql` — Consent, export, deletion, audit
- `000028_analytics.sql` — Events, materialized views
- `000029_spam_safety.sql` — IP blocks, censored words, reports

sqlc queries reference tables that must exist in migrations. Before writing queries, verify the migration has been applied:

```bash
go run ./cmd/migrate up
sqlc generate
```

If a migration number conflicts with an existing one, PostgreSQL will fail on the duplicate sequence entry. Always check the `schema_migrations` table or existing migration files before creating a new one.

## Lesson: SvelteKit Test File Naming

Test files in `src/routes` must NOT use the `+` prefix:

```
src/routes/tickets/page.test.ts    # CORRECT
src/routes/tickets/+page.test.ts   # WRONG — SvelteKit 2.68 reserves +
```

SvelteKit 2.68 treats files starting with `+` as route components. A `+page.test.ts` file would be interpreted as a route page, causing sync errors and build failures. This applies to all five JapaConnect SvelteKit apps (main, admin, control, learn, docs).
