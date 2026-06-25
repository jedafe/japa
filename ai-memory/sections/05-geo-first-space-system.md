# Section 05 — Geo-First Space System

**Status:** ✅ Locked (fully expanded post-audit)

## Core Model

Primary Space (exactly one per hierarchy level per user, changeable on relocation) plus unbounded Followed Spaces via the universal Follow System (Section 31). Joining a Space auto-follows it; following does not require joining.

```sql
CREATE TABLE spaces (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  parent_id       uuid REFERENCES spaces(id),
  space_type_id   uuid NOT NULL REFERENCES space_types(id),
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

-- Per-user per-space unread counter (Redis-backed, not DB)
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

Each axis is a text enum enforced at the middleware layer; a space moderator can override individual axes without changing the preset label:

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

## Hierarchy & Post Bubbling

Posts made in a City Space bubble up to its State Space, then to its Country Space, with a configurable depth cap (default 2 levels, stored in `system_settings` as `spaces.bubble_depth`). Bubbled posts appear with a "from [City]" label in parent feeds — they don't create duplicate `contents` rows; the `feeds` fan-out (Section 32) handles visibility at each level.

## Auto-Population (Wikipedia API + flagcdn.com)

```go
func SeedCountrySpace(ctx context.Context, country GeoCountry) error {
  // 1. Create space with geo_code
  spaceID := createSpace(ctx, country)
  // 2. Fetch Wikipedia extract for description (async, best-effort)
  enqueueTask(ctx, "space:auto_populate", spaceID) // low tier, Section 11
  return nil
}

// space:auto_populate task handler:
func PopulateSpaceFromExternalSources(ctx context.Context, spaceID uuid.UUID) error {
  space := getSpace(ctx, spaceID)
  // Logo: flagcdn.com/{iso_code}.svg for country spaces
  if space.Level == "country" {
    logoURL := "https://flagcdn.com/" + strings.ToLower(space.GeoCode) + ".svg"
    mediaID := downloadAndStoreMedia(ctx, logoURL, "space_logo")
    updateSpaceLogo(ctx, spaceID, mediaID)
  }
  // Description: Wikipedia API extract
  desc := fetchWikipediaExtract(ctx, space.Name)
  if desc != "" {
    updateSpaceDescription(ctx, spaceID, desc)
  }
  // Feature image: Pexels API (if configured)
  if pexelsKey := getConfig("PEXELS_API_KEY"); pexelsKey != "" {
    imgURL := fetchPexelsImage(ctx, space.Name+" city", pexelsKey)
    headerID := downloadAndStoreMedia(ctx, imgURL, "space_header")
    updateSpaceHeader(ctx, spaceID, headerID)
  }
  return nil
}
```

## See Also

Section 06 (space_types → custom_field_groups), Section 31 (Follow system), Section 32 (post bubbling via feed fan-out), Section 39 (University space type). ADR-005, ADR-020.
