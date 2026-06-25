# Section 06 — Universal Custom Field Architecture

**Status:** ✅ Locked (fully redesigned post-audit — see ADR-030)

## Philosophy

One field definition system underlies everything on the platform: Space Types, Content Types (Post Types), User Profiles, Organization Profiles, Forms, Tags, and Tickets all pull from the same `custom_field_groups` + `custom_fields` tables. Fields are never scattered across separate tables per entity type. Every field is RBAC-scoped — visibility, editability, and filterable-in-frontend are all per-UserGroup.

## Schema

```sql
CREATE TABLE custom_field_groups (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    uuid NOT NULL REFERENCES tenants(id),
  name         text NOT NULL,        -- 'Blog Post Fields', 'University Fields', 'User Profile Fields'
  slug         text NOT NULL,
  description  text,
  is_system    boolean NOT NULL DEFAULT false, -- seeded defaults, deletable only if no assignments
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE(tenant_id, slug)
);

CREATE TABLE custom_fields (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  group_id        uuid NOT NULL REFERENCES custom_field_groups(id) ON DELETE CASCADE,
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  label           text NOT NULL,
  key             text NOT NULL,       -- machine key, namespaced: 'blog.canonical_url'
  field_type      text NOT NULL,
  -- 'text'|'textarea'|'number'|'boolean'|'date'|'datetime'|'select'|'multiselect'
  -- |'url'|'email'|'phone'|'media'|'rich_text'|'platform_embed'|'color'|'rating'
  options         jsonb,               -- for select/multiselect: [{value, label}]
  embed_platforms text[],              -- for platform_embed: ['youtube','github','twitter','vimeo','spotify','figma','codepen','maps']
  placeholder     text,
  default_value   text,
  is_required     boolean NOT NULL DEFAULT false,
  is_system       boolean NOT NULL DEFAULT false,
  show_in_frontend boolean NOT NULL DEFAULT true,
  filterable      boolean NOT NULL DEFAULT false,
  sortable        boolean NOT NULL DEFAULT false,
  searchable      boolean NOT NULL DEFAULT false,
  position        int NOT NULL DEFAULT 0,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE(group_id, key)
);

-- Assignment: attach a field group to any entity class
CREATE TABLE custom_field_group_assignments (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  group_id       uuid NOT NULL REFERENCES custom_field_groups(id) ON DELETE CASCADE,
  entity_class   text NOT NULL,
  -- 'content_type:{slug}'|'space_type:{id}'|'user_profile'|'organization'|'form:{id}'|'tag'|'ticket_type:{id}'
  entity_id      uuid,                 -- NULL means applies to all of entity_class
  position       int NOT NULL DEFAULT 0,
  created_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE(group_id, entity_class, entity_id)
);

-- RBAC per field per UserGroup
CREATE TABLE custom_field_permissions (
  field_id       uuid NOT NULL REFERENCES custom_fields(id) ON DELETE CASCADE,
  user_group_id  uuid NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
  can_view       boolean NOT NULL DEFAULT true,
  can_edit       boolean NOT NULL DEFAULT false,
  PRIMARY KEY (field_id, user_group_id)
);

-- All field values stored in one polymorphic table (+ JSONB on entity)
-- For queryable fields we store in custom_field_values AND denormalize into field_values JSONB
CREATE TABLE custom_field_values (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  field_id     uuid NOT NULL REFERENCES custom_fields(id) ON DELETE CASCADE,
  entity_type  text NOT NULL,
  entity_id    uuid NOT NULL,
  value_text   text,
  value_number numeric,
  value_bool   boolean,
  value_date   timestamptz,
  value_json   jsonb,                  -- for media references, embed data
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE(field_id, entity_type, entity_id)
);
CREATE INDEX idx_cfv_entity ON custom_field_values (entity_type, entity_id);
CREATE INDEX idx_cfv_filterable ON custom_field_values (field_id, value_text) WHERE value_text IS NOT NULL;
```

## Platform Embed Field Type

The `platform_embed` field type handles URL → rich embed resolution for YouTube, GitHub, Twitter/X, Vimeo, Spotify, Figma, CodePen, Google Maps, and others:

```go
func ResolveEmbed(ctx context.Context, rawURL string, platforms []string) (*EmbedData, error) {
  parsed, _ := url.Parse(rawURL)
  host := strings.TrimPrefix(parsed.Hostname(), "www.")
  switch {
  case strings.HasSuffix(host, "youtube.com") || host == "youtu.be":
    return resolveYouTubeEmbed(ctx, rawURL)
  case host == "github.com":
    return resolveGitHubEmbed(ctx, rawURL) // repo card or gist
  case host == "twitter.com" || host == "x.com":
    return resolveOEmbedGeneric(ctx, "https://publish.twitter.com/oembed", rawURL)
  case host == "vimeo.com":
    return resolveOEmbedGeneric(ctx, "https://vimeo.com/api/oembed.json", rawURL)
  case host == "open.spotify.com":
    return resolveOEmbedGeneric(ctx, "https://open.spotify.com/oembed", rawURL)
  case host == "figma.com":
    return resolveFigmaEmbed(ctx, rawURL)
  case host == "codepen.io":
    return resolveOEmbedGeneric(ctx, "https://codepen.io/api/oembed", rawURL)
  case strings.HasSuffix(host, "google.com") && strings.Contains(parsed.Path, "/maps"):
    return resolveGoogleMapsEmbed(ctx, rawURL)
  }
  return nil, ErrUnsupportedPlatform
}
```

## Form Builder

Forms use the same `custom_fields` infrastructure — a Form is a `custom_field_group_assignment` with `entity_class = 'form:{form_id}'`:

```sql
CREATE TABLE forms (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  name            text NOT NULL,
  slug            text NOT NULL,
  description     text,
  is_multi_step   boolean NOT NULL DEFAULT false,
  submit_label    text NOT NULL DEFAULT 'Submit',
  success_message text,
  status          text NOT NULL DEFAULT 'active',
  is_system       boolean NOT NULL DEFAULT false,  -- Feedback, Report, Onboarding forms
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE(tenant_id, slug)
);

CREATE TABLE form_steps (
  id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  form_id  uuid NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  title    text NOT NULL,
  position int NOT NULL,
  UNIQUE(form_id, position)
);

-- Fields linked to a form step via custom_field_group_assignments where entity_id = step_id

CREATE TABLE form_submissions (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  form_id     uuid NOT NULL REFERENCES forms(id),
  user_id     uuid REFERENCES users(id),     -- null for anonymous
  ip_address  inet,
  field_values jsonb NOT NULL DEFAULT '{}',  -- key → value snapshot at submission time
  status      text NOT NULL DEFAULT 'new',   -- new|reviewed|actioned|spam
  created_at  timestamptz NOT NULL DEFAULT now()
);

-- Workflow triggers on form submission
CREATE TABLE form_submission_triggers (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  form_id     uuid NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  trigger_type text NOT NULL,
  -- 'assign_user_group'|'send_email'|'send_webhook'|'create_ticket'|'notify_admin'
  config      jsonb NOT NULL DEFAULT '{}',
  position    int NOT NULL DEFAULT 0
);
```

## Default Seeded Groups

| Group | Attached To | Key Fields |
|---|---|---|
| `user_profile_fields` | `user_profile` | Profession, Bio, Education, Japa Status, Gender, Social Links, Location Detail |
| `blog_post_fields` | `content_type:blog_post` | Canonical URL, Excerpt, Reading Level, Tags |
| `forum_fields` | `content_type:discussion` | Canonical URL, Allow Anonymous Reply |
| `space_fields` | all `space_type.*` | Rules, Featured Links, Contact Info |
| `university_fields` | `space_type:university` | Accreditation Body, Rankings, Programs |
| `organization_fields` | `organization` | Business Type, Founded Year, Employee Count, Social Links |
| `tag_fields` | `tag` | Wiki Content, Related Tags, External Link |
| `feedback_form` | `form:feedback` | Category, Description, Screenshot |
| `report_form` | `form:report` | Reason, Evidence, Additional Notes |

## Japa Status

A `select` custom field in `user_profile_fields` group:

```go
JapaStatusOptions = []FieldOption{
  {Value: "newly_japa",         Label: "Newly Japa (< 1 year)"},
  {Value: "settled",            Label: "Settled Abroad"},
  {Value: "about_to_japa",      Label: "About to Japa"},
  {Value: "planning",           Label: "Planning to Relocate"},
  {Value: "looking_jobs",       Label: "Looking for Jobs Abroad"},
  {Value: "looking_study",      Label: "Looking to Study Abroad"},
  {Value: "returning",          Label: "Considering Returning Home"},
  {Value: "remote_worker",      Label: "Remote Worker (Nigeria-based)"},
}
```

Admin can add/edit/remove options from the Admin Panel without a code deploy.

## See Also

ADR-030. Section 05 (space_types → field groups), Section 09 (onboarding uses user_profile_fields), Section 14 (content type field groups), Section 38 (ticket type field groups), Section 39 (university field group).
