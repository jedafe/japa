# JapaConnect Backend API Surface — Reference

## Tech Stack
- **Backend**: Go 1.26, Chi router, pgx/v5, sqlc codegen
- **Database**: PostgreSQL 18 + pgvector, 130 migrations, ~180+ tables
- **Cache/Queue**: Valkey (Redis-compatible), Asynq background jobs
- **Search**: Meilisearch
- **Storage**: SeaweedFS (S3-compatible) + imgproxy
- **Auth**: JWT (KeyRotation), OAuth (Google/GitHub/Apple/Facebook), TOTP 2FA
- **Base URL**: `/api/v1/`

## Architecture Pattern
Every domain follows: **Handler → Service → Store → gen.Queries**
- Reader/writer pool separation (primary/replica)
- Tenant-scoped via `tenant_id` on every query
- Standard envelope: `{"data": ...}` / `{"error": {"code": "...", "message": "..."}}`
- Cursor-based and offset pagination

## Middleware Stack (all /api/v1/* requests)
1. RequestID → Recovery → Logger → CORS → GeoIP → IPBlock
2. Tenant resolution (from Host header)
3. Rate limit (loose: 300/min baseline)
4. Ban enforcement (Valkey-backed)
5. Bot detection

## Auth Pattern
- `auth.RequireAuth(kr, vk)` — required, returns 401
- `auth.OptionalAuth(kr, vk)` — passes through silently
- JWT contains: uid, tid, jti
- Token revocation checked via Valkey per-request
- Brute force protection on login

## Rate Limit Tiers
| Tier | Requests | Window | Used On |
|------|----------|--------|---------|
| tight | 10 | 1 min | /auth/*, /ws |
| moderate | 60 | 1 min | /content POST, /search, /blog POST |
| loose | 300 | 1 min | All /api/v1/* (baseline) |

## RBAC
- `rbac.RequirePermission(resolver, "perm.key")` — returns 403
- Three-tier: users → user_groups → user_roles → permissions
- Valkey L2 + in-memory L1 cache

## Feature Gating
8 features behind flags (return 404 when disabled):
`feature.dm`, `feature.forum`, `feature.blog`, `feature.ratings_reviews`, `feature.comparison`, `feature.professional`, `feature.wallet`, `feature.ai_chat`

---

## Complete Endpoint Inventory

### Auth (`/api/v1/auth/*`) — tight rate limit
| Method | Path | Notes |
|--------|------|-------|
| POST | /auth/register | |
| POST | /auth/login | Brute force protected |
| POST | /auth/refresh | |
| POST | /auth/logout | |
| POST | /auth/forgot-password | |
| GET | /auth/reset-password | |
| POST | /auth/reset-password | |
| GET | /auth/verify-email | |
| POST | /auth/resend-verification | |
| POST | /auth/oauth/{provider}/initiate | Google, GitHub, Apple, Facebook |
| GET | /auth/oauth/{provider}/callback | |

### TOTP 2FA
| Method | Path |
|--------|------|
| GET | /me/totp/status |
| POST | /me/totp/enable |
| POST | /me/totp/confirm |
| DELETE | /me/totp |

### User Profile & Account
| Method | Path | Notes |
|--------|------|-------|
| GET | /me | Current user |
| PATCH | /me/profile | |
| PATCH | /me/account | |
| GET/PUT | /me/preferences | |
| POST | /me/change-password | |
| GET/PATCH | /me/privacy | |
| GET/DELETE | /me/sessions/* | |
| GET/DELETE | /me/connected-accounts/* | |
| GET | /me/content | My content |
| GET | /me/drafts | |
| GET | /me/scheduled | |
| GET | /me/bookmarks | |
| GET | /me/analytics | |
| POST | /me/deactivate | |
| DELETE | /me | GDPR deletion |
| GET | /users | Admin: list users |
| GET | /users/{username} | Public profile |

### Spaces (40+ endpoints)
| Method | Path | Notes |
|--------|------|-------|
| GET | /spaces | List all |
| GET | /me/spaces | My spaces |
| GET | /me/spaces/moderated | |
| GET | /spaces/{slug} | Single space (SpaceResolver) |
| POST | /spaces | RBAC: spaces.create |
| PATCH | /spaces/{id} | RBAC: spaces.edit |
| DELETE | /spaces/{id} | RBAC: spaces.delete |
| POST | /spaces/{slug}/join | Toggle membership |
| DELETE | /spaces/{slug}/leave | |
| GET | /spaces/{slug}/members | |
| POST/DELETE | /spaces/{slug}/follow | |
| POST/DELETE | /spaces/{slug}/mute | |
| GET | /spaces/{slug}/content | |
| GET | /spaces/{slug}/pinned | |
| POST/DELETE | /spaces/{slug}/pinned | RBAC: spaces.moderate |
| POST | /spaces/{id}/report | |
| CRUD | /spaces/{slug}/rules/* | RBAC: spaces.moderate |
| POST/DELETE | /spaces/{slug}/moderators/* | |
| GET | /spaces/{id}/search | |
| GET | /spaces/search | moderate rate |
| GET | /spaces/{id}/analytics | |
| GET | /space-types | |
| Admin | /admin/space-types/* | RBAC: space_types.* |
| Admin | /admin/spaces/{id}/join-requests/* | |
| Admin | /admin/spaces/{id}/status | |

### Content (30+ endpoints)
| Method | Path | Notes |
|--------|------|-------|
| POST | /content | RBAC: content.create, moderate rate |
| GET | /content | List |
| GET | /content/{slug} | Single |
| GET | /content-types | |
| PATCH | /content/{id} | RBAC: content.edit |
| DELETE | /content/{id} | RBAC: content.delete |
| POST | /content/{id}/publish | |
| POST | /content/{id}/archive | |
| POST | /content/{id}/schedule | |
| POST | /content/{id}/view | Record view |
| GET | /content/{id}/revisions | |
| POST/DELETE | /content/{id}/pin | RBAC: content.pin |
| POST/DELETE/GET | /content/{id}/reactions/* | |
| POST/DELETE/GET | /content/{id}/bookmark | |
| POST | /content/{id}/share | |
| GET | /content/{id}/related | pgvector similarity |
| GET/POST | /content/{id}/discussions | |
| GET | /content/jsonld-templates | |
| Admin | /admin/content | RBAC: admin.moderation |
| Admin | /admin/content/{id}/remove | |
| Admin | /admin/content/{id}/restore | |

### Comments (8 endpoints)
| Method | Path |
|--------|------|
| GET | /content/{id}/comments |
| POST | /content/{id}/comments |
| PATCH | /comments/{id} |
| DELETE | /comments/{id} |
| POST/DELETE | /comments/{id}/vote |
| POST | /comments/{id}/report |
| POST/DELETE | /comments/{id}/pin |

### Forum (25+ endpoints, gated: feature.forum)
| Method | Path | Notes |
|--------|------|-------|
| POST | /forum | Create thread |
| GET | /forum | List threads |
| GET | /forum/{slug} | Single thread |
| PATCH/DELETE | /forum/{id} | |
| POST/DELETE | /forum/{id}/solve | |
| POST/GET | /forum/{id}/replies | |
| PATCH/DELETE | /forum/replies/{id} | |
| POST/DELETE | /forum/{id}/vote | |
| CRUD | /forum/polls/* | |
| POST | /forum/polls/{id}/vote | |
| CRUD | /forum/tags/* | |
| POST/DELETE | /forum/{id}/tags/{tagId} | |
| GET/POST/DELETE | /forum/bookmarks/* | |

### Blog (25+ endpoints, gated: feature.blog)
| Method | Path | Notes |
|--------|------|-------|
| GET | /blog | List posts |
| POST | /blog | RBAC: content.create |
| GET | /blog/{slug} | Single post |
| PATCH/DELETE | /blog/{id} | |
| POST | /blog/{id}/submit-for-review | |
| POST | /blog/{id}/feature | RBAC: blog.feature |
| CRUD | /blog/series/* | |
| POST/DELETE | /blog/series/{id}/posts/* | |
| PATCH | /blog/series/{id}/posts/reorder | |
| CRUD | /blog/reviews/* | |
| GET | /blog/featured | |
| GET | /blog/feed.xml | RSS |

### Follows (9 endpoints)
| Method | Path |
|--------|------|
| POST | /follows |
| GET | /me/following |
| GET | /me/followed-tags |
| GET | /users/{user_id}/followers |
| GET | /users/{user_id}/following |
| GET | /users/{user_id}/mutual/{other} |
| GET | /users/{user_id}/follow-status |
| GET | /spaces/{space_id}/followers |
| POST/DELETE | /users/{id}/follow |

### Interactions — Blocks & Mutes (7 endpoints)
| Method | Path |
|--------|------|
| POST/DELETE | /users/{user_id}/block |
| GET | /me/blocked-users |
| POST/DELETE | /users/{user_id}/mute |
| GET | /users/{user_id}/mute-status |
| POST | /users/{id}/report |
| GET | /me/muted-users |
| GET | /me/muted-spaces |

### Notifications (15+ endpoints)
| Method | Path |
|--------|------|
| GET | /me/notifications |
| GET | /me/notifications/unread-count |
| POST | /me/notifications/{id}/read |
| POST | /me/notifications/read-all |
| DELETE | /me/notifications/{id} |
| GET/PATCH | /me/notification-preferences |
| PATCH | /me/notification-preferences/{type} |
| GET | /me/notification-types |
| GET | /push/vapid-key | Public |
| POST | /me/push-tokens |
| DELETE | /me/push-tokens/{id} |

### Feed (6 endpoints)
| Method | Path | Auth |
|--------|------|------|
| GET | /feed/card-style | Public |
| GET | /explore | Public |
| GET | /feed | Required |
| GET | /feed/space/{space_id} | Required |
| GET | /spaces/{space_id}/feed | Required |
| GET | /feed/profile/{user_id} | Required |

### Trending & Discovery (8 endpoints)
| Method | Path | Auth |
|--------|------|------|
| GET | /trending/content | Public |
| GET | /trending/tags | Public |
| GET | /trending/spaces | Public |
| GET | /trending/users | Public |
| GET | /suggestions/users | Required |
| GET | /suggestions/spaces | Required |
| GET | /suggestions/tags | Required |
| GET | /suggestions/content | Optional |

### Search (6 endpoints, moderate rate)
| Method | Path |
|--------|------|
| GET | /search?q=&type=&limit= |
| GET | /search/autocomplete?q= |
| GET | /spaces/search?q= |
| GET | /spaces/{id}/search?q= |
| GET | /tags/{slug}/search?q= |
| GET | /admin/search/analytics |

### Tags (10 endpoints)
| Method | Path |
|--------|------|
| GET | /tags | Public |
| GET | /tags/{slug} | Public |
| GET | /tags/{slug}/content | Public |
| POST/DELETE | /tags/{slug}/follow |
| GET | /tags/{slug}/followers |
| Admin | /admin/tags CRUD |

### Media (15+ endpoints)
| Method | Path |
|--------|------|
| POST | /media/upload |
| POST | /media/confirm/{id} |
| GET/PATCH/DELETE | /media/{id} |
| GET | /media |
| GET | /me/media |
| POST | /media/bulk-delete |
| Admin | /admin/media/* (quarantine, approve, reject) |

### WebSocket & DM (16 endpoints, gated: feature.dm)
| Method | Path | Notes |
|--------|------|-------|
| POST | /auth/ws-ticket | Get auth ticket |
| GET | /ws?ticket= | WebSocket upgrade (tight rate) |
| GET | /ws/online | Online users |
| GET/POST | /dm/conversations | List/Create |
| GET/POST | /dm/conversations/{id}/messages | List/Send |
| PATCH | /dm/conversations/{id}/messages/{mid} | Edit |
| DELETE | /dm/conversations/{id}/messages/{mid} | Delete |
| POST | /dm/conversations/{id}/read | Mark read |
| POST | /dm/requests/{id}/accept | |
| POST | /dm/requests/{id}/decline | |
| POST | /dm/requests/{id}/block | |
| PUT | /dm/privacy | |
| GET | /dm/requests | |

### Payments (18+ endpoints)
| Method | Path | Notes |
|--------|------|-------|
| POST | /payments/intent | Idempotency key required |
| GET | /me/payments | |
| GET | /me/payments/{id} | |
| POST | /payments/{id}/refund | RBAC: payments.refund |
| POST | /payments/webhook/paystack | Public webhook |
| POST | /payments/webhook/stripe | Public webhook |
| POST | /payments/webhook/nowpayments | Public webhook |
| POST | /subscriptions | Idempotency |
| GET | /subscriptions | |
| GET | /me/subscription | |
| POST | /me/subscription/cancel | Idempotency |
| POST | /me/subscription/reactivate | |
| POST | /payments/discount-codes/validate | |
| GET | /pricing/plans | Public |
| GET | /payment-providers | Public |
| Admin | /admin/discount-codes/* | |

### Wallet (15+ endpoints, gated: feature.wallet)
| Method | Path |
|--------|------|
| GET | /me/wallet/balance |
| GET | /me/wallet/ledger |
| POST | /me/wallet/convert |
| Admin | /admin/wallet/* (rules, settings, adjustments) |

### AI (20+ endpoints)
| Method | Path | Notes |
|--------|------|-------|
| POST | /ai/generate | RBAC: ai.generate |
| POST | /ai/chat | Gated: feature.ai_chat |
| POST | /ai/assist/improve | |
| POST | /ai/assist/summarize | |
| POST | /ai/assist/translate | |
| POST | /ai/assist/generate-tags | |
| POST | /ai/assist/generate-og | |
| POST/GET | /ai/hitl/* | RBAC: ai.hitl.review |
| GET | /ai/categories | |
| GET | /ai/conversations | |
| GET | /ai/conversations/{id} | |
| Admin | /admin/ai/tools/* | RBAC: ai.tools.manage |
| Admin | /admin/ai/context/* | |
| Admin | /admin/ai/usage | |
| Admin | /admin/ai/models | |

### Geo (6 endpoints)
| Method | Path | Auth |
|--------|------|------|
| GET | /geo/me | Public |
| GET | /geo/detect | Public |
| GET | /geo/lookup | Required |
| GET | /geo/countries | Public |
| GET | /geo/countries/{code}/states | Public |
| GET | /geo/states/{id}/cities | Public |

### Reviews (9 endpoints, gated: feature.ratings_reviews)
| Method | Path |
|--------|------|
| POST | /reviews/{entity_type}/{entity_id} |
| GET | /reviews |
| GET | /reviews/{entity_type}/{entity_id} |
| GET | /reviews/{entity_type}/{entity_id}/aggregate |
| GET | /reviews/{id} |
| PATCH/DELETE | /reviews/{id} |
| POST | /reviews/{id}/flag |

### Organizations (15 endpoints)
| Method | Path |
|--------|------|
| CRUD | /organizations/* |
| Members | /organizations/{id}/members/* |
| Invitations | /organizations/{id}/invitations/* |
| Claim | /claim-ownership |

### Professional Profiles (35+ endpoints, gated: feature.professional)
Full CRUD on profile types, profiles, members, invitations, relocations, documents, verifiers.

### Comparisons (25+ endpoints, gated: feature.comparison)
Full CRUD on types, access, comparisons, items, dimensions, values.

### Tickets (20 endpoints)
User portal + admin management under /tickets/* and /admin/tickets/*.

### Learning (20+ endpoints)
| Method | Path | Notes |
|--------|------|-------|
| GET | /learn/tracks | Published tracks |
| GET | /learn/tracks/{slug} | Track detail |
| GET | /learn/tracks/{ts}/modules/{ms}/lessons/{ls} | Lesson by slug |
| POST | /learn/tracks/{id}/enroll | |
| POST | /learn/lessons/{id}/progress | |
| POST | /learn/lessons/{id}/complete | |
| GET | /learn/quizzes/{id} | |
| POST | /learn/quizzes/{id}/submit | |
| GET | /certificates/{id} | |
| GET | /certificates/{id}/download | |
| GET | /me/certificates | |
| GET | /me/enrollments | |
| Admin | /admin/learn/* CRUD | |

### Custom Fields & Forms (25+ endpoints)
EAV system for dynamic fields on any entity. Forms with submissions.

### Page Builder (25+ endpoints)
| Method | Path | Notes |
|--------|------|-------|
| GET | /pages/{slug} | Public page |
| Admin | /admin/pagebuilder/* | Blocks, templates, themes, pages |

### Docs & Knowledge Base (12 endpoints)
| Method | Path |
|--------|------|
| GET | /docs/{audience} |
| GET | /docs/{audience}/{slug} |
| GET | /docs/nav |
| GET | /docs/search |
| Admin | /admin/docs/* CRUD |

### Advertising (20+ endpoints)
Campaigns, slot types, boosts, public ad serving.

### Onboarding (5 endpoints)
| Method | Path |
|--------|------|
| GET | /onboarding/status |
| PUT | /onboarding/step |
| POST | /onboarding/complete |
| POST | /onboarding/recommendations |
| POST | /onboarding/tour/dismiss |

### i18n (8 endpoints)
| Method | Path |
|--------|------|
| GET | /i18n/packs |
| GET | /i18n/packs/{lang_code} |
| Admin | /admin/i18n/* |

### GDPR (12+ endpoints)
Consent, data export, account deletion, audit logs.

### Analytics (12 endpoints)
| Method | Path |
|--------|------|
| POST | /analytics/events |
| GET | /analytics/dashboard |
| Admin | /admin/analytics/* |

### Webhooks (10+ endpoints)
Admin CRUD for outbound webhooks + deliveries + retry.

### Newsletter (10 endpoints)
Admin subscribers + campaigns + send.

### Coupons & Affiliates
Public coupons, click tracking, affiliate enrollment, links, earnings.

### Developer Portal / API Keys
| Method | Path |
|--------|------|
| GET | /developer/swagger |
| GET | /developer/scopes |
| CRUD | /me/api-keys/* |
| CRUD | /me/api-keys/{id}/ip-allowlist/* |

### RBAC Admin (15+ endpoints)
User groups, permissions, roles CRUD. GET /me/permissions.

### Control Panel (`/control/*`) — Super admin
Tenants, system settings, feature flags, health, queue monitor, cron, cache, security, API keys, AI budget, audit logs, deployments.

---

## Key Entity Relationships

### Users
```
tenants ─1:N─> users ─1:1─> user_profiles
                    ├─1:N─> refresh_tokens
                    ├─1:1─> totp_credentials
                    ├─1:N─> oauth_accounts
                    └─1:N─> api_keys ─1:N─> api_key_ip_allowlist
```

### RBAC
```
users ─M:N─> user_groups ─M:N─> user_roles ─M:N─> permissions
```

### Spaces
```
space_types ─1:N─> spaces ─self-ref─> parent_id
                     ├─1:N─> space_rules
                     ├─M:N─> space_members
                     ├─1:N─> space_moderators
                     └─1:N─> space_pinned_posts ─FK─> contents
```

### Content
```
contents (discriminator: content_type)
  ├─1:N─> content_revisions
  ├─1:N─> content_tags ─FK─> tags
  ├─1:N─> content_views
  ├─1:1─> content_scores (pgvector embedding)
  ├─M:N─> reactions (polymorphic)
  └─soft delete via deleted_at
```

### Comments
```
contents ─1:N─> comments (self-referencing: parent_id)
                    └─1:N─> comment_votes
```

### Commerce
```
payments ─> subscriptions, refunds, discount_codes, webhooks
wallets ─> wallet_transactions
coupons ─> coupon_events
```

### Learning
```
learning_tracks ─1:N─> learning_modules ─1:N─> learning_lessons
                                                         └─1:N─> lesson_progress
enrollments (user × track)
learning_certificates
```

---

## Existing Frontend Apps
| App | Port | Purpose |
|-----|------|---------|
| apps/main/ | 3000 | Main user-facing app (all user features) |
| apps/admin/ | 3001 | Admin dashboard |
| apps/control/ | 3002 | Internal control panel |
| apps/learn/ | 3003 | Learning platform |
| apps/docs/ | 3004 | Documentation site |

All use: SvelteKit 2 + Svelte 5 (runes) + Tailwind CSS 4 + bits-ui + Bun
