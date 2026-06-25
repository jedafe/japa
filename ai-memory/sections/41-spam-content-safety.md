# Section 41 — Spam Prevention & Content Safety

**Status:** ✅ Locked

## Multi-Layer Defence

```
Layer 1: Registration protection    — Cloudflare Turnstile CAPTCHA
Layer 2: Honeypot fields            — hidden form fields on all public forms
Layer 3: New-user link restriction  — no external links in first N posts (admin-configurable)
Layer 4: Duplicate content hash     — SHA-256 of normalised body, checked before save
Layer 5: IP blocking                — CIDR-level block table, checked in middleware
Layer 6: User blocking              — mutual visibility suppression
Layer 7: Censored words             — server-side text filter, client-side preview indicator
Layer 8: NSFW image detection       — client-side NSFW.js pre-check, server-side PhotoDNA hash
Layer 9: AI moderation scan         — optional async scan via moderation_agent (Section 20)
Layer 10: Manual moderation         — reporting queue (Section 22), admin review
```

## Schema

```sql
-- IP Blocking
CREATE TABLE ip_blocks (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    uuid NOT NULL REFERENCES tenants(id),
  cidr         inet NOT NULL,          -- e.g. '105.112.0.0/16'
  block_type   text NOT NULL DEFAULT 'full',
  -- 'full'|'registration'|'posting'|'api'
  reason       text,
  blocked_by   uuid REFERENCES users(id),
  expires_at   timestamptz,            -- NULL = permanent
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ip_blocks_cidr ON ip_blocks USING gist (cidr inet_ops);

-- User-to-User Blocking
CREATE TABLE user_blocks (
  blocker_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  blocked_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (blocker_id, blocked_id)
);
CREATE INDEX idx_user_blocks_blocked ON user_blocks (blocked_id);

-- Censored Words
CREATE TABLE censored_words (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id   uuid NOT NULL REFERENCES tenants(id),
  pattern     text NOT NULL,           -- exact word or regex if starts with '^'
  action      text NOT NULL DEFAULT 'block',  -- 'block'|'replace'|'flag'
  replacement text,                    -- used when action='replace'
  created_at  timestamptz NOT NULL DEFAULT now()
);
-- Cached in Valkey: jc:censored_words:{tenant_id} → JSON array, TTL 1h

-- Duplicate Content Detection
-- SHA-256 of strip_markdown(content).toLowerCase().replaceAll(/\s+/g, ' ').trim()
-- stored in contents.content_hash, UNIQUE constraint per tenant
ALTER TABLE contents ADD COLUMN content_hash text;
CREATE UNIQUE INDEX idx_contents_hash ON contents (tenant_id, content_hash) WHERE content_hash IS NOT NULL;
```

## IP Block Middleware

```go
func IPBlockMiddleware(queries *db.Queries) func(http.Handler) http.Handler {
  return func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
      ip := net.ParseIP(realIP(r))
      if isBlocked, blockType := checkIPBlock(r.Context(), queries, ip); isBlocked {
        switch blockType {
        case "full":
          http.Error(w, "Access denied", http.StatusForbidden)
          return
        case "registration":
          // Only block registration endpoint — stored in ctx for handler to check
          ctx := context.WithValue(r.Context(), IPBlockTypeKey, "registration")
          next.ServeHTTP(w, r.WithContext(ctx))
          return
        }
      }
      next.ServeHTTP(w, r)
    })
  }
}
```

## User Block Effects

When user A blocks user B:
- B's content is hidden from A's feeds and search results
- A's content is hidden from B's feeds and search results
- B cannot follow A, comment on A's posts, send A a DM request
- B's @mention of A produces no notification to A
- Mutual invisibility enforced at query level (JOIN user_blocks check)

```go
// Applied to every content list query for authenticated users:
func ExcludeBlockedUsers(ctx context.Context, userID uuid.UUID, q sq.SelectBuilder) sq.SelectBuilder {
  return q.Where(`
    NOT EXISTS (
      SELECT 1 FROM user_blocks ub
      WHERE (ub.blocker_id = ? AND ub.blocked_id = c.author_user_id)
         OR (ub.blocker_id = c.author_user_id AND ub.blocked_id = ?)
    )`, userID, userID)
}
```

## Censored Word Filter

```go
func CheckCensoredWords(ctx context.Context, tenantID uuid.UUID, text string) (CensorResult, error) {
  patterns := getCachedCensoredWords(ctx, tenantID)
  for _, p := range patterns {
    if matchesPattern(text, p.Pattern) {
      switch p.Action {
      case "block":
        return CensorResult{Blocked: true, Reason: "contains prohibited content"}, nil
      case "replace":
        text = replacePattern(text, p.Pattern, p.Replacement)
      case "flag":
        // Post goes to moderation queue with auto-flag
        return CensorResult{Flagged: true, FlagReason: "censored word detected"}, nil
      }
    }
  }
  return CensorResult{CleanText: text}, nil
}
```

## See Also

Section 22 (moderation queue), Section 37 (GeoIP feeds IP block enrichment), ADR-022 (rate limiting layer), ADR-041.
