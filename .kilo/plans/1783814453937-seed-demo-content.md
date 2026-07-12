# Seed Demo Content Plan

## Goal
Extend `cmd/seed/main.go` to create demo content across all major features so the feed and feature pages look populated for testing.

## Scope — 10+ items per feature

### Features to seed (with target counts)

| Feature | Table(s) | Count | Notes |
|---------|----------|-------|-------|
| **Users** | `users`, `user_profiles` | 8 demo users | Needed as authors for all content |
| **Tags** | `tags` | 15 | Tech/relocation topics for content_tag pills |
| **Blog posts** | `contents`, `content_scores` | 12 | `content_type='blog_post'`, published, across spaces |
| **Forum threads** | `forum_threads`, `forum_replies`, `forum_tags`, `forum_thread_tags` | 12 threads + 30 replies | With votes, solved status |
| **Comparisons** | `contents`, `comparison_items`, `comparison_dimensions`, `comparison_values` | 10 | `content_type='comparison'` |
| **Reviews** | `reviews`, `review_aggregates` | 12 | Polymorphic on `organization` entity type |
| **Learning tracks** | `learning_tracks`, `learning_modules`, `learning_lessons` | 10 tracks + 20 modules + 40 lessons | Published, beginner-intermediate |
| **Tickets** | `ticket_types`, `tickets`, `ticket_comments` | 10 tickets + 20 comments | Various statuses |
| **Organizations** | `organizations`, `organization_members` | 5 | Nigerian diaspora businesses/services |
| **Content bookmarks** | `bookmarks` | 20 | Spread across demo users |
| **Follows** | `follows` | 20 | User→user, user→space |
| **Comments** | `comments` | 20 | On blog_post content |

### Key constraints
- **content_scores** must be inserted for every `contents` row (feed queries `JOIN content_scores`)
- `contents.slug` must be unique per `tenant_id`
- `contents.status = 'published'` and `visibility = 'public'` for feed visibility
- Forum threads use their own `slug` column (not `contents.slug`)
- All IDs use `gen_random_uuid()` — the seed will capture returned IDs for FK references
- The seed must be idempotent (check existence before insert)

## Architecture decision: Extend Go seed vs SQL migration

**Decision: Extend `cmd/seed/main.go`** (not a new SQL migration)

Rationale:
- The Go seed already has the idempotent pattern (check admin exists, skip)
- UUIDs from inserted rows are needed as FK references (comparisons need content_id → comparison_items, etc.)
- SQL migrations can't easily do "insert, get ID, use ID in next insert" across tables
- Go code is testable and can be re-run safely

## Implementation plan

### Step 1: Refactor seed to support multiple users
Add a `seedUsers()` function that creates 8 demo users with profiles. Each user gets a unique email (`user1@japaconnect.com` through `user8@japaconnect.com`, password `Demo1234!`). Each user is added to the `members` group.

### Step 2: Seed tags
Create 15 tags with relocation/tech topics (e.g., "visa-tips", "uk-healthcare", "nigerian-food-abroad", "tech-jobs", "remittance", "housing", "education", "banking", "career", "entrepreneurship", "culture-shock", "driving-license", "insurance", "tax-planning", "mental-health").

### Step 3: Seed blog posts (12)
Create 12 blog_post contents distributed across the 4 spaces. Each gets:
- `contents` row with `status='published'`, `visibility='public'`, realistic `body` and `body_html`
- `content_scores` row with random `vote_score`, `comment_count`, `view_count`, `bookmark_count`
- `content_tags` links to 2-3 tags
- `read_time_minutes` computed from body length

### Step 4: Seed forum threads (12) + replies (30)
Create 12 forum threads across spaces, each with 2-5 replies. Include:
- 3 threads marked `solved=true`
- Forum votes on threads and replies
- Forum tags linked to threads
- `content_id` FK to `contents` for each thread (forum threads reference content)

### Step 5: Seed comparisons (10)
Create 10 comparison contents (`content_type='comparison'`) with:
- 3-5 items per comparison (e.g., "UK vs Canada vs Australia" with comparison_items)
- 3-5 dimensions per comparison (cost, healthcare, weather, etc.)
- Values populated for each dimension×item intersection

### Step 6: Seed reviews (12)
Create 12 reviews on `organization` entity type, each with:
- Random rating 1-5
- Title and body text
- `review_aggregates` upserted after all reviews

### Step 7: Seed learning tracks (10) + modules (20) + lessons (40)
Create 10 learning tracks:
- 2 modules each, 2 lessons each module
- Published, varied difficulty levels
- Realistic titles: "UK Visa Guide", "Canadian PR Pathways", "US H1B Process", etc.

### Step 8: Seed tickets (10) + comments (20)
Create 10 tickets across 2-3 ticket types:
- Various statuses (open, in_progress, resolved, closed)
- 2 comments per ticket on average

### Step 9: Seed organizations (5)
Create 5 organizations with members:
- Nigerian diaspora businesses/services
- Owner + 2-3 members each

### Step 10: Seed bookmarks (20) + follows (20) + comments (20)
- 20 bookmarks spread across demo users on content entities
- 20 follows (user→user, user→space)
- 20 comments on blog_post content items

## File changes

| File | Change |
|------|--------|
| `cmd/seed/main.go` | Add `seedDemoContent()` function called after admin seed. Add helper functions for each feature. |

No new files needed — all logic goes into the existing seed command.

## Validation
1. `go build ./cmd/seed/` must pass
2. After running seed, verify counts:
   - `SELECT count(*) FROM contents WHERE status='published'` → ≥42 (12 blog + 10 comparison + 20+ from others)
   - `SELECT count(*) FROM forum_threads` → ≥12
   - `SELECT count(*) FROM learning_tracks WHERE is_published=true` → ≥10
   - `SELECT count(*) FROM tickets` → ≥10
   - `SELECT count(*) FROM reviews` → ≥12
   - `SELECT count(*) FROM content_scores` → ≥12
   - `SELECT count(*) FROM tags` → ≥15
   - `SELECT count(*) FROM follows` → ≥20
3. Feed pages should show populated cards
