# Section 33 — Professional Profiles

**Status:** ✅ Locked

## Decided

Professional Profiles are distinct from personal user profiles and from Organization Profiles (Section 07). A Professional Profile represents a structured public-facing entity — an agency, consultancy, practice, or brand — that a user owns and can post as. Users switch authorship context when creating content.

```sql
CREATE TABLE professional_profile_types (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id   uuid NOT NULL REFERENCES tenants(id),
  name        text NOT NULL,   -- 'Organization', 'Agency', 'Consultancy', 'Brand', 'Practice'
  slug        text NOT NULL,
  icon        text,
  field_group_id uuid REFERENCES custom_field_groups(id),
  is_system   boolean NOT NULL DEFAULT false,
  UNIQUE(tenant_id, slug)
);

CREATE TABLE professional_profiles (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  owner_user_id   uuid NOT NULL REFERENCES users(id),
  type_id         uuid NOT NULL REFERENCES professional_profile_types(id),
  name            text NOT NULL,
  slug            text NOT NULL UNIQUE,
  tagline         text,
  description     text,
  logo_media_id   uuid REFERENCES media(id),
  header_media_id uuid REFERENCES media(id),
  website_url     text,
  verified        boolean NOT NULL DEFAULT false,
  verification_type text,              -- 'embassy'|'business'|'notable'
  follower_count  int NOT NULL DEFAULT 0,
  status          text NOT NULL DEFAULT 'active',
  field_values    jsonb NOT NULL DEFAULT '{}',
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE professional_profile_members (
  profile_id   uuid NOT NULL REFERENCES professional_profiles(id) ON DELETE CASCADE,
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role         text NOT NULL DEFAULT 'member',  -- 'owner'|'admin'|'editor'|'member'
  invited_by   uuid REFERENCES users(id),
  joined_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (profile_id, user_id)
);

CREATE TABLE professional_profile_invitations (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  profile_id  uuid NOT NULL REFERENCES professional_profiles(id) ON DELETE CASCADE,
  email       text NOT NULL,
  role        text NOT NULL DEFAULT 'member',
  invited_by  uuid REFERENCES users(id),
  token       text NOT NULL UNIQUE,
  expires_at  timestamptz NOT NULL,
  accepted_at timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);
```

## Authorship Switching

When a user who owns or belongs to a Professional Profile creates content, they choose their authorship context:

```go
type AuthorContext struct {
  Type string    // 'user' | 'professional_profile'
  ID   uuid.UUID
}

// Contents table has both author_user_id (always set) and
// author_profile_id (set when posting as a professional profile)
// Display name and avatar come from whichever is set
```

## Claim Ownership Integration

The Claim Ownership flow from Section 07 attaches a Professional Profile to an existing unclaimed listing. Once claimed and verified, the professional profile owner can manage the listing's page and post on behalf of it.

## See Also

Section 07 (Claim Ownership), Section 31 (professional profiles are followable), Section 33 feeds into Section 32 (authorship shown in feed cards). ADR-031.
