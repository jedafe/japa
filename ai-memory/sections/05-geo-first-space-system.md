# Section 05 — Geo-First Space System

**Status:** ✅ Locked (expanded in session 5 — space membership, moderators, multi-parent spaces, creation quota, cross-form auto-suggest)

## Core Model

Primary Space (exactly one per hierarchy level per user, changeable on relocation) plus unbounded Followed Spaces via the universal Follow System (Section 31). Joining a Space auto-follows it; following does not require joining.

**Three Primary Space Levels (system-seeded, cannot be deleted):**
- Country — auto-seeded from open geo dataset (ADR-020)
- State / Province — auto-seeded
- City — admin-created on demand, never auto-seeded

**Custom Spaces** — user-created, must be linked to at least one primary space (country/state/city). Enforced at the API handler; creation is rejected if no primary parent is selected.

## Schema

```sql
CREATE TABLE spaces (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  space_type_id   uuid NOT NULL REFERENCES space_types(id),
  creator_user_id uuid REFERENCES users(id) ON DELETE SET NULL,  -- NULL for system-seeded spaces
  name            text NOT NULL,
  slug            text NOT NULL,
  description     text,
  wiki_content    jsonb,                  -- TipTap JSON for rich wiki tab
  logo_media_id   uuid REFERENCES media(id),
  header_image_id uuid REFERENCES media(id),
  geo_code        text,                   -- ISO code for country/state
  level           text NOT NULL,          -- 'country'|'state'|'city'|'custom'
  member_count    int NOT NULL DEFAULT 0,
  post_count      int NOT NULL DEFAULT 0,
  follower_count  int NOT NULL DEFAULT 0,
  visibility      text NOT NULL DEFAULT 'public',
  posting_perm    text NOT NULL DEFAULT 'members',
  replying_perm   text NOT NULL DEFAULT 'members',
  commenting_perm text NOT NULL DEFAULT 'members',
  joining_perm    text NOT NULL DEFAULT 'open',
  status          text NOT NULL DEFAULT 'active',
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE(tenant_id, slug)
);

-- Many-to-many: custom space → one or more primary (country/state/city) spaces
-- Primary spaces have no rows here (they ARE parents, never have parents)
CREATE TABLE space_parent_spaces (
  space_id        uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  parent_space_id uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  PRIMARY KEY (space_id, parent_space_id),
  CHECK (space_id != parent_space_id)
);
CREATE INDEX idx_space_parent_spaces_parent ON space_parent_spaces (parent_space_id);

-- Explicit membership — separate from following
-- Following a space does not create a row here; joining does
CREATE TABLE space_members (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id  uuid NOT NULL REFERENCES tenants(id),
  space_id   uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  joined_at  timestamptz NOT NULL DEFAULT now(),
  invited_by uuid REFERENCES users(id) ON DELETE SET NULL,
  UNIQUE(space_id, user_id)
);
CREATE INDEX idx_space_members_user ON space_members (user_id, space_id);

-- Moderators — distinct from membership
-- Admin-assigned: no membership requirement; space-owner-assigned: user must be in space_members
CREATE TABLE space_moderators (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id           uuid NOT NULL REFERENCES tenants(id),
  space_id            uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  assigned_by         text NOT NULL CHECK (assigned_by IN ('site_admin', 'space_owner')),
  assigned_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at          timestamptz NOT NULL DEFAULT now(),
  UNIQUE(space_id, user_id)
);

CREATE TABLE space_rules (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  title       text NOT NULL,
  body        text NOT NULL,
  position    int NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE space_pinned_posts (
  space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  content_id  uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
  position    int NOT NULL CHECK (position BETWEEN 1 AND 5),
  pinned_by   uuid REFERENCES users(id),
  pinned_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (space_id, content_id)
);

-- Per-user per-space unread counter (Valkey-backed, not DB)
-- Key: jc:space_unread:{user_id}:{space_id} → INT
-- Incremented on new post fan-out, reset to 0 on visit
```

## Space Types & Permission Presets

```sql
CREATE TABLE space_types (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  name            text NOT NULL,          -- 'Country', 'City', 'University', 'Community', etc.
  slug            text NOT NULL,
  description     text,
  icon            text,
  field_group_id  uuid REFERENCES custom_field_groups(id), -- Section 06
  is_system       boolean NOT NULL DEFAULT false,           -- cannot be deleted
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE(tenant_id, slug)
);
```

**Seven default permission presets (seeded, admin-editable):**

| Preset | visibility | posting | replying | commenting | joining |
|---|---|---|---|---|---|
| `open_community` | public | members | members | members | open |
| `geo_space` | public | members | members | members | open |
| `curated` | public | moderators | members | members | request |
| `private` | members | members | members | members | invite |
| `read_only` | public | moderators | moderators | moderators | open |
| `verified_only` | public | verified | members | members | open |
| `admin_only` | members | admins | admins | admins | invite |

## Five Permission Axes

```go
type SpacePermAxes struct {
  Visibility  string // 'public' | 'members' | 'private'
  Posting     string // 'open' | 'members' | 'verified' | 'moderators' | 'admins'
  Replying    string // same options as Posting
  Commenting  string // same options as Posting
  Joining     string // 'open' | 'request' | 'invite'
}

func CanPostInSpace(ctx context.Context, u *User, s *Space) bool {
  switch s.PostingPerm {
  case "open":       return true
  case "members":    return isMember(ctx, u.ID, s.ID)
  case "verified":   return u.Verified && isMember(ctx, u.ID, s.ID)
  case "moderators": return isModerator(ctx, u.ID, s.ID)
  case "admins":     return HasPermission(ctx, u, "spaces.moderate.any")
  default:           return false
  }
}
```

## Space Creation Quota

Space creation is gated by the `spaces.create` RBAC permission node. Quantity limits are per-UserGroup:

```sql
-- max_spaces_creatable column on user_groups
-- 0 = cannot create; -1 = unlimited; N = capped at N
ALTER TABLE user_groups ADD COLUMN max_spaces_creatable int NOT NULL DEFAULT 0;
```

Enforcement at the API handler:

```go
func (h *SpaceHandler) CreateSpace(w http.ResponseWriter, r *http.Request) {
    userID := MustUserID(r)

    // Check permission node
    if !HasPermission(r.Context(), userID, "spaces.create") {
        renderError(w, http.StatusForbidden, "not permitted to create spaces")
        return
    }

    // Check quota
    group := getUserPrimaryGroup(r.Context(), userID)
    if group.MaxSpacesCreatable >= 0 { // -1 = unlimited
        count, _ := queries.CountSpacesCreatedBy(r.Context(), userID)
        if int(count) >= group.MaxSpacesCreatable {
            renderError(w, http.StatusForbidden, "space creation quota reached")
            return
        }
    }

    // Validate at least one primary parent selected
    if len(body.ParentSpaceIDs) == 0 {
        renderError(w, http.StatusBadRequest, "custom spaces must belong to at least one primary space")
        return
    }
    // ...
}
```

## Space Moderator Assignment Rules

```go
// AssignModerator — POST /api/v1/spaces/:id/moderators
func (h *SpaceHandler) AssignModerator(w http.ResponseWriter, r *http.Request) {
    requesterID := MustUserID(r)
    spaceID     := chi.URLParam(r, "id")

    assignedBy := "space_owner"
    if HasPermission(r.Context(), requesterID, "spaces.moderate.any") {
        assignedBy = "site_admin"
    } else {
        // Space owner path: requester must be the creator
        space, _ := queries.GetSpace(r.Context(), spaceID)
        if space.CreatorUserID != requesterID {
            renderError(w, http.StatusForbidden, "only the space creator or site admin can assign moderators")
            return
        }
        // Space-owner path: target must be a member of the space
        if !isMember(r.Context(), body.UserID, spaceID) {
            renderError(w, http.StatusBadRequest, "space-owner-assigned moderators must be space members")
            return
        }
    }
    // Site-admin path: no membership requirement — proceed directly
    // ...
}
```

**Bot / AI Moderators:** Any `users` row where `is_bot = true` can be assigned as a space moderator via the same endpoint. Bot accounts are created by admins in the Admin Panel. The `users` table includes `is_bot boolean NOT NULL DEFAULT false`.

## Primary Space Auto-Suggest During Space Creation

When a user types a custom space name, a debounced Meilisearch search returns relevant primary spaces (country/state level only) for the parent space selector:

```go
// GET /api/v1/spaces/suggest-parents?q=Nigeria+Workers+in+Brazil
// Searches the spaces Meilisearch index, filtered to level IN ('country','state','city')
// Returns top 5 ranked primary spaces for the user to select from

func SuggestParentSpaces(w http.ResponseWriter, r *http.Request) {
    q := r.URL.Query().Get("q")
    results, _ := meili.Index("spaces").Search(q, &meilisearch.SearchRequest{
        Filter: "level IN [country, state, city] AND status = active",
        Limit:  5,
    })
    renderJSON(w, http.StatusOK, results.Hits)
}
```

**Example:** User types "Nigeria Workers in Brazil" → Meilisearch returns Nigeria (country) and Brazil (country) as top suggestions. User selects both. The space is then linked to both via `space_parent_spaces`.

## Space Field Auto-Suggest Across the Platform

A reusable `<SpaceCombobox>` Svelte component powers the Space selector in every form on the platform (registration, post creation, profile edit, etc.):

```svelte
<!-- SpaceCombobox.svelte — shared component, Bits UI Combobox foundation -->
<!-- Props: multiple (bool), initialValue, onSelect, filter (level filter) -->
<!-- Behavior:
     1. On focus: show user's joined spaces + primary spaces first (from store)
     2. On keystroke (debounced 200ms): search Meilisearch /api/v1/spaces/search?q=...
     3. Highlight user's own spaces with a member badge
     4. Returns array of selected space IDs to parent via onSelect callback
-->
export let multiple: boolean = false;
export let filter: string = '';           // e.g. 'level:country' to restrict to countries only
export let initialValue: string[] = [];
export let onSelect: (spaceIds: string[]) => void;
```

The API endpoint backing it:
```go
// GET /api/v1/spaces/search?q=london&filter=level:city
// Auth: optional — public spaces visible to all; members-only spaces only visible if user is a member
func SearchSpaces(w http.ResponseWriter, r *http.Request) {
    // Returns spaces sorted by: (1) spaces user is a member of, (2) followed spaces, (3) public spaces
}
```

## Hierarchy & Post Bubbling

Posts made in a custom Space bubble up to its parent primary spaces (up to `spaces.bubble_depth` levels, default 2, stored in `system_settings`). Bubbled posts appear with a "from [Space name]" label in parent feeds — they don't create duplicate `contents` rows; the `feeds` fan-out (Section 32) handles visibility at each level.

## Auto-Population (Wikipedia API + flagcdn.com)

```go
func SeedCountrySpace(ctx context.Context, country GeoCountry) error {
  spaceID := createSpace(ctx, country)
  enqueueTask(ctx, "space:auto_populate", spaceID) // low tier, Section 11
  return nil
}

func PopulateSpaceFromExternalSources(ctx context.Context, spaceID uuid.UUID) error {
  space := getSpace(ctx, spaceID)
  if space.Level == "country" {
    logoURL := "https://flagcdn.com/" + strings.ToLower(space.GeoCode) + ".svg"
    mediaID := downloadAndStoreMedia(ctx, logoURL, "space_logo")
    updateSpaceLogo(ctx, spaceID, mediaID)
  }
  desc := fetchWikipediaExtract(ctx, space.Name)
  if desc != "" { updateSpaceDescription(ctx, spaceID, desc) }
  if pexelsKey := getConfig("PEXELS_API_KEY"); pexelsKey != "" {
    imgURL := fetchPexelsImage(ctx, space.Name+" city", pexelsKey)
    headerID := downloadAndStoreMedia(ctx, imgURL, "space_header")
    updateSpaceHeader(ctx, spaceID, headerID)
  }
  return nil
}
```

## See Also

Section 06 (space_types → custom_field_groups), Section 31 (Follow system), Section 32 (post bubbling via feed fan-out, multi-space fan-out), Section 39 (University space type), Section 14 (content_spaces junction table). ADR-005, ADR-020, ADR-049.
