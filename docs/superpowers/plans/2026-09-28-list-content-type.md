# List Content Type (Listicle) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a first-class `list` content type ("List" — BuzzFeed-style listicles) to the universal content system: backend registration + permission gate + item validation, a dedicated `ListBuilder` composer, canonical `/list/{slug}` + `/lists` public routes, internal-content-embed hydration, ItemList JSON-LD, and i18n.

**Architecture:** §14 behavior-pack pattern — `contents.content_type = 'list'`, structured items in `field_values` JSONB, ordinary `body`/`body_html` serialized by the composer (numbers baked into markup, embed placeholders hydrated client-side). No new API routes, no DB schema change, no review queue. Creation gated by new RBAC node `list.create` (super_admin only by default) + public setting `list.enabled`; `list.max_items` caps item count.

**Tech Stack:** Go (chi, pgx, sqlc-generated code — never hand-edited), PostgreSQL migrations, SvelteKit 2 + Svelte 5 runes (`apps/main`), Bun toolchain only (Rule 18).

**Spec:** `docs/superpowers/specs/2026-09-28-list-content-type-design.md` — read it first.

## Global Constraints

- **JEOS before code** (AGENTS.md): Task 1 lands all `.jeos` amendments before any implementation.
- Never hand-edit `internal/db/gen/*` (Rule 2). This plan adds **no** new SQL queries — everything reuses existing generated code.
- API errors use `errpkg.WriteError` with `snake_case` codes (Rule 5). Codes introduced: `list_feature_disabled`, `list_permission_required`, `invalid_list_mode`, `invalid_list_items`, `too_many_items`.
- `body_html` goes through the existing `SanitizeHTML` whitelist; the only sanitizer change is `data-content-slug` on `div`.
- `list.enabled` is a **public system setting** (settings catalog, `Public: true`) — NOT a `feature_flags` row. Do not add it to `internal/features/registry.go` and do not add `/list(s)` to `routeFeatureFlags` (LeftNav reads the `features` map only; a settings-keyed entry would permanently hide nav items). Frontend surfaces gate on `siteSettings['list.enabled']` — the `video.enabled` pattern. Missing = enabled per spec.
- Frontend data comes from APIs (Rule 16): type labels from `GET /content-types`, permissions from `locals.user.permissions`, picker results from `GET /content?q=`/`GET /link-preview`/`POST /videos/resolve`/`GET /media/{id}`.
- No AI-attribution strings anywhere (Rule 17). Commit on `main` only.
- The working tree has unrelated modifications (apps/control, internal/intelligence, etc.) — stage only files this plan touches.

**Spec reconciliations applied in this plan:**
1. `field_values` gains `intro_html` (spec schema lists `list_mode` + `items` only; "intro from `body`" would lose rich formatting on draft restore — `field_values` is the editing source of truth, so intro lives there).
2. `agent.content_type.list` needs **no** Go change — `allowedContentTypes` derives the node name dynamically from the type catalog (`internal/ai/tools_content.go:66`). Only the seed migration + §03 doc are required. The spec's file list naming `tools_content_bulk.go`/`agent.go` is a no-op.
3. ItemList `itemListElement` cannot come from the flat `{{field}}` interpolator (string values only, no arrays) — the plan adds a post-interpolation injection step in `GetContent` that builds `itemListElement` + `numberOfItems` from `field_values.items`.
4. **`embed.resolved.embed_url` is dormant in v1.** `GET /link-preview` returns `{url,title,description,image,site_name,favicon}` — no oEmbed/`embed_url` — and `customfields.EmbedResolver` (oEmbed for Twitter/Vimeo/Spotify/CodePen) has no HTTP route. Since the spec mandates no new API routes, `kind:'url'` embeds always serialize as `link-preview` placeholders. NOTE: a raw `<iframe src>` in `body_html` does **not** survive `SanitizeHTML` today — `allowedAttributes` has no `iframe` entry, so `src` is stripped even for `isApprovedIframe` hosts (pre-existing gap; nothing in the platform emits raw iframes into body — all rich embeds use hydrated divs). The serializer still emits `<iframe>` when `resolved.embed_url` is set so a future `GET /embed/resolve` + `allowedAttributes["iframe"]` whitelist amendment can light it up without a schema change; until then the composer never populates `embed_url`. oEmbed-capable video platforms (YouTube/TikTok/Instagram/Vimeo per `video.import.allowed_platforms`) are covered by the Video embed tab via `POST /videos/resolve`.
5. `/lists` is modeled on **`/guides`**, not `/stories` — `/guides` consumes the identical `GET /content?type=` endpoint with `GuideListItem` rows, `apiGetEnvelope` (the response is `{data:[]}` with **no** `meta.total`), cursor (`created_at|id`) pagination for `latest`, `page_offset` for `popular`, single-page `featured` tab, and `attachAuthorUsernames`.

---

### Task 1: JEOS amendments (spec-first, required by AGENTS.md)

**Files:**
- Modify: `.jeos/sections/14-content-type-system.md`
- Modify: `.jeos/sections/03-rbac.md`
- Modify: `.jeos/conventions/system-settings-catalog.md`
- Modify: `.jeos/conventions/api-routes-catalog.md`

- [ ] **Step 1: §14 — add `list` to the built-in type list and append a "List Content Type System" amendment**

Find the built-in content-type list in §14 and add `list` ("List"). Append an amendment section covering:
- `field_values` schema: `{ list_mode: 'numbered'|'countdown'|'unnumbered', intro_html: string, items: [{ heading?, body_html?, media_id?, embed?, source_url?, credit? }] }` where `embed = { kind: 'content'|'url'|'video', content_id?, content_slug?, snapshot?, url?, resolved?, video_source_id? }`.
- Contract: composer serializes `body` (plain text incl. items) and `body_html` (intro + `<div class="list-item" id="item-N">` markup, `<span class="li-num">N.</span>` baked numbers, embed placeholders). `field_values` is the editing source; `body_html` is the render source.
- Embed placeholder contract: `div[data-type="content-embed"][data-content-slug]` (internal), `div[data-type="link-preview"][data-url]` (external), `div.video-embed[data-video-source-id]` (video), `<img>` (media), `<iframe>` (approved oEmbed hosts).
- Permission gate `list.create` on `POST /content`; settings `list.enabled` / `list.max_items`.
- Route family `/list/{slug}` + `/lists`.

- [ ] **Step 2: §03 — document the new nodes**

Add to the permission catalog:
- `list.create` — "Create list (listicle) content via composer / POST /content". Default grants: `super_admin`/`super_admin_role` only. Not granted to `member`.
- `agent.content_type.list` — "AI Agent — create list content". Granted to every role holding `agent.content_type.blog_post` (keeps the agent node family complete).

- [ ] **Step 3: settings catalog**

Add to `system-settings-catalog.md`:

| Key | Type | Default | Public | Description |
|-----|------|---------|--------|-------------|
| `list.enabled` | bool | `true` | yes | Killswitch for the list content type, `/lists` index, and composer entry |
| `list.max_items` | int | `50` | no | Max items per list; enforced on POST/PATCH /content |

- [ ] **Step 4: routes catalog**

Add a note that `list` flows through the universal content routes (`POST /content`, `PATCH /content/{id}`, `GET /content?type=list`, `GET /content/{slug}`, `GET /content-types`) — no new API routes.

- [ ] **Step 5: Commit**

```bash
git add .jeos/sections/14-content-type-system.md .jeos/sections/03-rbac.md .jeos/conventions/system-settings-catalog.md .jeos/conventions/api-routes-catalog.md
git commit -m "docs(§14/§03): List content type JEOS amendments"
```

---

### Task 2: Migration 000749 — permissions + ItemList template

**Files:**
- Create: `migrations/000749_list_content_type.up.sql`
- Create: `migrations/000749_list_content_type.down.sql`

**Interfaces:**
- Produces: permission keys `list.create`, `agent.content_type.list`; role grants; active `content_type_jsonld_templates` row for `content_type='list'`.
- Role slugs verified against baseline seed: `super_admin`, `super_admin_role`, `admin-group-role`, `member` (migrations/000735_baseline_seed.up.sql:36640+).

- [ ] **Step 1: Write the up migration**

```sql
-- 000749_list_content_type.up.sql
-- List (listicle) content type — §14 amendment.
-- Seeds list.create (super_admin only; admins opt other groups in via the
-- RBAC admin UI) and agent.content_type.list (granted to every role that
-- already holds the agent.content_type family, mirroring 000561). Also seeds
-- the ItemList JSON-LD template per §14's admin-template design.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('list.create', 'Create list (listicle) content via the composer or POST /content'),
    ('agent.content_type.list', 'AI Agent — create list content')
ON CONFLICT (key) DO NOTHING;

-- list.create → super-admin roles only. NOT granted to member/admin-group-role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key = 'list.create'
ON CONFLICT DO NOTHING;

-- agent.content_type.list → every role already holding the agent.content_type
-- family (000561 granted it to member + super_admin). Derive the grant set
-- from an existing family member so future roles stay consistent.
INSERT INTO role_permissions (role_id, permission_id)
SELECT DISTINCT rp.role_id, p.id
FROM role_permissions rp
JOIN permissions src ON src.id = rp.permission_id AND src.key = 'agent.content_type.blog_post'
JOIN permissions p ON p.key = 'agent.content_type.list'
ON CONFLICT DO NOTHING;

-- ItemList JSON-LD template. itemListElement/numberOfItems are injected at
-- render time by the content handler (the {{field}} interpolator only emits
-- strings, so list items cannot be expressed in the template itself).
INSERT INTO content_type_jsonld_templates (content_type, template, is_active) VALUES (
    'list',
    '{"@context":"https://schema.org","@type":"ItemList","name":"{{title}}","description":"{{description}}","url":"{{canonical_url}}","image":"{{cover_image_url}}","author":{"@type":"Person","name":"{{author_name}}"},"datePublished":"{{published_at}}","dateModified":"{{updated_at}}"}'::jsonb,
    true
);

COMMIT;
```

- [ ] **Step 2: Write the down migration**

```sql
-- 000749_list_content_type.down.sql
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN ('list.create','agent.content_type.list'));
DELETE FROM permissions WHERE key IN ('list.create','agent.content_type.list');
DELETE FROM content_type_jsonld_templates WHERE content_type = 'list';
COMMIT;
```

- [ ] **Step 3: Run migration**

```bash
make migrate
```
Expected: applies cleanly. Verify:
```bash
psql $JC_DATABASE_URL -c "SELECT key FROM permissions WHERE key LIKE 'list%' OR key='agent.content_type.list';"
```
Expected: two rows (`list.create`, `agent.content_type.list`).

- [ ] **Step 4: Commit**

```bash
git add migrations/000749_list_content_type.up.sql migrations/000749_list_content_type.down.sql
git commit -m "Seed list.create + agent.content_type.list permissions and ItemList JSON-LD template"
```

---

### Task 3: Backend — register the `list` type + settings catalog

**Files:**
- Modify: `internal/content/content.go`
- Modify: `internal/control/settings_catalog.go`
- Test: `internal/content/content_test.go` (create if absent; check for existing test file first)

**Interfaces:**
- Produces: `content.TypeList = "list"`; `IsValidContentType("list") == true`; `ContentTypeLabels["list"] == "List"`; `GET /content-types` advertises `{value:"list", label:"List"}` automatically (it iterates `ContentTypes()`); `list.enabled`/`list.max_items` in the settings catalog.

- [ ] **Step 1: Write the failing test**

In `internal/content/content_test.go` (or the package's existing registry test file — check `ls internal/content/*_test.go` first):

```go
func TestListContentTypeRegistered(t *testing.T) {
	if !IsValidContentType(TypeList) {
		t.Fatal("TypeList not in validContentTypes")
	}
	if TypeList != "list" {
		t.Fatalf("TypeList = %q, want list", TypeList)
	}
	if got := ContentTypeLabels[TypeList]; got != "List" {
		t.Fatalf("label = %q, want List", got)
	}
	found := false
	for _, ct := range ContentTypes() {
		if ct == TypeList {
			found = true
		}
	}
	if !found {
		t.Fatal("TypeList missing from ContentTypes()")
	}
}
```

Run: `go test ./internal/content/ -run TestListContentTypeRegistered -v`
Expected: FAIL (type not registered).

- [ ] **Step 2: Register the type**

In `internal/content/content.go` (~line 96-120 const block), after `TypeForumThread`:

```go
	// List (listicle) content type — BuzzFeed-style numbered/countdown/
	// unnumbered item lists with per-item embeds. §14 list amendment.
	TypeList = "list"
```

Add `TypeList: true` to `validContentTypes` (~line 155) and `TypeList: "List"` to `ContentTypeLabels` (~line 204).

- [ ] **Step 3: Add catalog settings**

In `internal/control/settings_catalog.go`, add near the video block (end of map, ~line 699):

```go
	// List (listicle) content type — §14 list amendment.
	// Public: the /create composer and /lists index gate on this setting
	// (same role as video.enabled). Missing = enabled.
	"list.enabled":   {Type: "bool", DefaultValue: "true", Public: true, Description: "Killswitch for the list content type, /lists index, and /create composer.", Panel: "admin"},
	"list.max_items": {Type: "int", DefaultValue: "50", Min: 1, Max: 500, Description: "Maximum number of items allowed in a list content.", Panel: "admin"},
```

- [ ] **Step 4: Verify**

```bash
go test ./internal/content/ -run TestListContentTypeRegistered -v && go build ./internal/content/ ./internal/control/
```
Expected: test PASS, build clean.

- [ ] **Step 5: Commit**

```bash
git add internal/content/content.go internal/control/settings_catalog.go internal/content/content_test.go
git commit -m "Register list content type and list.* settings (§14)"
```

---

### Task 4: Backend — `list.create` + `list.enabled` gates and `field_values` validation

**Files:**
- Create: `internal/content/list.go` — validation/sanitization + JSON-LD injection helpers
- Modify: `internal/content/handlers.go` — `CreateContent` (perm + enabled + validate), `UpdateContent` (validate), `GetContent` (ItemList injection)

**Interfaces:**
- `validateListFieldValues(fv json.RawMessage, maxItems int) (json.RawMessage, *listValidationError)` — returns the sanitized field_values JSON (items' `body_html` re-sanitized) or a `*listValidationError{Code, Message}` for `errpkg.WriteError`. The caller resolves `maxItems` via `h.settings.GetIntOr(ctx, "list.max_items", 50)` (`config.Settings` is a concrete struct that tests can't inject values into — passing the resolved int keeps the helper unit-testable; `&config.Settings{}` yields the fallback). Handles `nil`/empty `fv` as valid.
- `injectListItemListElements(jsonLd []byte, fv json.RawMessage) []byte` — merges `itemListElement`/`numberOfItems` into a resolved ItemList template.

- [ ] **Step 1: Write failing validation tests first**

Create `internal/content/list_test.go`:

```go
func TestValidateListFieldValues(t *testing.T) {
	cases := []struct {
		name    string
		fv      string
		wantErr string // expected code; "" = valid
	}{
		{"empty", ``, ""},
		{"empty object", `{}`, ""},
		{"numbered ok", `{"list_mode":"numbered","items":[{"heading":"A"}]}`, ""},
		{"bad mode", `{"list_mode":"banana","items":[]}`, "invalid_list_mode"},
		{"items not array", `{"items":{}}`, "invalid_list_items"},
		{"bad media uuid", `{"items":[{"media_id":"nope"}]}`, "invalid_list_items"},
		{"media and embed", `{"items":[{"media_id":"` + uuid.New().String() + `","embed":{"kind":"url","url":"https://x.co"}}]}`, "invalid_list_items"},
		{"bad embed kind", `{"items":[{"embed":{"kind":"portal"}}]}`, "invalid_list_items"},
		{"content embed missing slug", `{"items":[{"embed":{"kind":"content","content_id":"` + uuid.New().String() + `"}}]}`, "invalid_list_items"},
		{"url embed bad scheme", `{"items":[{"embed":{"kind":"url","url":"javascript:alert(1)"}}]}`, "invalid_list_items"},
		{"bad source_url", `{"items":[{"source_url":"ftp://x"}]}`, "invalid_list_items"},
	}
	for _, tc := range cases {
		_, err := validateListFieldValues(json.RawMessage(tc.fv), 50)
		if tc.wantErr == "" && err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}
		if tc.wantErr != "" && (err == nil || err.Code != tc.wantErr) {
			t.Errorf("%s: want %s, got %v", tc.name, tc.wantErr, err)
		}
	}
}

func TestValidateListFieldValuesMaxItems(t *testing.T) {
	fv := `{"items":[{"heading":"a"},{"heading":"b"},{"heading":"c"}]}`
	if _, err := validateListFieldValues(json.RawMessage(fv), 2); err == nil || err.Code != "too_many_items" {
		t.Fatalf("want too_many_items, got %v", err)
	}
	if _, err := validateListFieldValues(json.RawMessage(fv), 3); err != nil {
		t.Fatalf("3 items at max 3 should pass, got %v", err)
	}
}

func TestListItemHTMLSanitized(t *testing.T) {
	fv := `{"items":[{"body_html":"<p onclick='x()'>hi</p><script>bad()</script>"}]}`
	out, err := validateListFieldValues(json.RawMessage(fv), 50)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("onclick")) || bytes.Contains(out, []byte("script")) {
		t.Fatal("item body_html not sanitized")
	}
}
```

Run: `go test ./internal/content/ -run 'TestValidateList' -v`
Expected: FAIL (function doesn't exist).

- [ ] **Step 2: Implement `internal/content/list.go`**

```go
package content

// Ref: docs/superpowers/specs/2026-09-28-list-content-type-design.md
//      .jeos/sections/14-content-type-system.md (List amendment)

const listDefaultMaxItems = 50

type listValidationError struct {
	Code    string
	Message string
}

func (e *listValidationError) Error() string { return e.Message }

type listItemEmbed struct {
	Kind          string          `json:"kind"`
	ContentID     string          `json:"content_id,omitempty"`
	ContentSlug   string          `json:"content_slug,omitempty"`
	Snapshot      json.RawMessage `json:"snapshot,omitempty"`
	URL           string          `json:"url,omitempty"`
	Resolved      json.RawMessage `json:"resolved,omitempty"`
	VideoSourceID string          `json:"video_source_id,omitempty"`
}

type listItem struct {
	Heading   string         `json:"heading,omitempty"`
	BodyHTML  string         `json:"body_html,omitempty"`
	MediaID   string         `json:"media_id,omitempty"`
	Embed     *listItemEmbed `json:"embed,omitempty"`
	SourceURL string         `json:"source_url,omitempty"`
	Credit    string         `json:"credit,omitempty"`
}

// validateListFieldValues validates and normalizes the `list` field_values
// payload: list_mode enum, item cap (system_settings list.max_items, resolved
// by the caller), per-item body_html sanitization, UUID/embed-kind checks,
// http(s)-only URLs.
func validateListFieldValues(raw json.RawMessage, maxItems int) (json.RawMessage, *listValidationError) {
	var fv map[string]json.RawMessage
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &fv); err != nil {
			return nil, &listValidationError{"invalid_list_items", "field_values must be an object"}
		}
	} else {
		fv = map[string]json.RawMessage{}
	}

	if modeRaw, ok := fv["list_mode"]; ok {
		var mode string
		if err := json.Unmarshal(modeRaw, &mode); err != nil ||
			(mode != "numbered" && mode != "countdown" && mode != "unnumbered") {
			return nil, &listValidationError{"invalid_list_mode", "list_mode must be numbered, countdown, or unnumbered"}
		}
	}

	var items []listItem
	if itemsRaw, ok := fv["items"]; ok {
		if err := json.Unmarshal(itemsRaw, &items); err != nil {
			return nil, &listValidationError{"invalid_list_items", "items must be an array"}
		}
	}
	if len(items) > maxItems {
		return nil, &listValidationError{"too_many_items", fmt.Sprintf("list supports at most %d items", maxItems)}
	}
	for i := range items {
		if err := validateListItem(&items[i]); err != nil {
			return nil, err
		}
	}
	// items were unmarshalled into a detached slice — write the (sanitized)
	// slice back into the map before re-marshalling the whole payload.
	if enc, err := json.Marshal(items); err == nil {
		fv["items"] = enc
	}
	out, err := json.Marshal(fv)
	if err != nil {
		return nil, &listValidationError{"invalid_list_items", err.Error()}
	}
	return out, nil
}
```

`validateListItem` rules (implement in the same file):
- `len(Heading) > 300` → error; `len(Credit) > 300` → error.
- `BodyHTML` → `items[i].BodyHTML = SanitizeHTML(items[i].BodyHTML)` (write back).
- `MediaID` and `Embed` are mutually exclusive — both set → `invalid_list_items`.
- `MediaID` non-empty must parse via `pgtype.UUID`/`uuid.Parse`; likewise `embed.content_id`, `embed.video_source_id`.
- `embed.kind` ∈ `{"content","url","video"}`; `content` requires `content_id` + `content_slug`; `url` requires `url`; `video` requires `video_source_id`.
- `embed.url`, `source_url`, and `embed.resolved.embed_url`/`resolved.url` when present must be `http:`/`https:` (parse with `net/url`, reject other schemes).

- [ ] **Step 3: Wire gates into `CreateContent`**

In `internal/content/handlers.go` `CreateContent`, immediately after the existing `story.create` block (handlers.go ~line 906-917), add:

```go
	// §14 list amendment: list creation requires the list.create node
	// (super_admin wildcard or explicit grant) and respects the public
	// list.enabled killswitch.
	if input.ContentType == TypeList {
		if h.settings != nil && !h.settings.GetBoolOr(r.Context(), "list.enabled", true) {
			errpkg.WriteError(w, http.StatusForbidden, "list_feature_disabled",
				"List creation is currently disabled")
			return
		}
		allowed := false
		if resolver := rbac.FromContext(r.Context()); resolver != nil {
			allowed = resolver.IsSuperAdmin(r.Context(), user.UserID) ||
				resolver.HasPermission(r.Context(), user.UserID, "list.create")
		}
		if !allowed {
			errpkg.WriteError(w, http.StatusForbidden, "list_permission_required",
				"You do not have permission to create lists")
			return
		}
		maxItems := listDefaultMaxItems
		if h.settings != nil {
			maxItems = h.settings.GetIntOr(r.Context(), "list.max_items", listDefaultMaxItems)
		}
		fv, vErr := validateListFieldValues(input.FieldValues, maxItems)
		if vErr != nil {
			errpkg.WriteError(w, http.StatusBadRequest, vErr.Code, vErr.Message)
			return
		}
		input.FieldValues = fv
	}
```

- [ ] **Step 4: Wire validation into `UpdateContent`**

In `UpdateContent` (handlers.go ~line 1547): `prevContent` is loaded at ~line 1612 (`h.Service.GetContent`) and `updateInput` is built at ~line 1656 — the edit-permission check happens later (~1754), so insert the block **after the prevContent default-fill block (~line 1633) and before `updateInput`**:

```go
	if prevContent.ContentType == TypeList && len(input.FieldValues) > 0 {
		maxItems := listDefaultMaxItems
		if h.settings != nil {
			maxItems = h.settings.GetIntOr(r.Context(), "list.max_items", listDefaultMaxItems)
		}
		fv, vErr := validateListFieldValues(input.FieldValues, maxItems)
		if vErr != nil {
			errpkg.WriteError(w, http.StatusBadRequest, vErr.Code, vErr.Message)
			return
		}
		input.FieldValues = fv
	}
```

Locate the exact anchor: find where `prevContent` is first available in `UpdateContent` (it is fetched before the permission checks around line 1750-1790). Place the block right before the `updateInput` struct is built (field used at ~line 1661 is on the earlier `input` decode — verify the variable names match: `input.FieldValues`).

- [ ] **Step 5: Inject `itemListElement` into the ItemList JSON-LD**

In `GetContent` (handlers.go ~line 1386), replace the jsonLd block with:

```go
	if jsonLd, jErr := h.Service.ResolveJSONLD(r.Context(), content.ContentType, contentJSONLDFields(content)); jErr == nil && len(jsonLd) > 0 {
		if content.ContentType == TypeList {
			jsonLd = injectListItemListElements(jsonLd, content.FieldValues)
		}
		resp["json_ld"] = json.RawMessage(jsonLd)
	} else if jErr != nil {
		slog.Warn("content: JSON-LD template resolution failed", "slug", slug, "content_type", content.ContentType, "error", jErr)
	}
```

Implement `injectListItemListElements` in `list.go`:

```go
// injectListItemListElements merges itemListElement/numberOfItems derived from
// field_values.items into a resolved ItemList JSON-LD document. For
// 'countdown' lists the array order is reversed so position 1 is the top-ranked
// item (the list's last element). Best-effort: any parse failure returns the
// input unchanged.
func injectListItemListElements(jsonLd []byte, fv json.RawMessage) []byte {
	var doc map[string]any
	if err := json.Unmarshal(jsonLd, &doc); err != nil {
		return jsonLd
	}
	var parsed struct {
		ListMode string     `json:"list_mode"`
		Items    []listItem `json:"items"`
	}
	if err := json.Unmarshal(fv, &parsed); err != nil || len(parsed.Items) == 0 {
		return jsonLd
	}
	type element struct {
		Type     string `json:"@type"`
		Position int    `json:"position"`
		Name     string `json:"name,omitempty"`
		URL      string `json:"url,omitempty"`
	}
	els := make([]element, 0, len(parsed.Items))
	for i, it := range parsed.Items {
		pos := i + 1
		idx := i
		if parsed.ListMode == "countdown" {
			idx = len(parsed.Items) - 1 - i
			it = parsed.Items[idx]
		}
		els = append(els, element{Type: "ListItem", Position: pos, Name: it.Heading})
	}
	doc["itemListElement"] = els
	doc["numberOfItems"] = len(parsed.Items)
	out, err := json.Marshal(doc)
	if err != nil {
		return jsonLd
	}
	return out
}
```

- [ ] **Step 6: Build + test**

```bash
go build ./internal/content/ && go test ./internal/content/ -run 'TestValidateList|TestListItem|TestInjectList' -v
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/content/list.go internal/content/list_test.go internal/content/handlers.go
git commit -m "Gate list creation on list.create/list.enabled and validate list field_values (§14)"
```

---

### Task 5: Backend — sanitizer whitelist + tests

**Files:**
- Modify: `internal/content/sanitize.go`
- Test: `internal/content/sanitize_test.go` (check it exists first)

- [ ] **Step 1: Failing test**

```go
func TestSanitizeHTMLKeepsContentEmbedAttrs(t *testing.T) {
	in := `<div data-type="content-embed" data-content-slug="my-guide"><a href="/guide/my-guide">card</a></div>`
	out := SanitizeHTML(in)
	if !strings.Contains(out, `data-content-slug="my-guide"`) {
		t.Fatalf("data-content-slug stripped: %s", out)
	}
}

func TestSanitizeHTMLKeepsVideoEmbedSourceID(t *testing.T) {
	in := `<div class="video-embed" data-video-source-id="123e4567-e89b-12d3-a456-426614174000"></div>`
	out := SanitizeHTML(in)
	if !strings.Contains(out, `data-video-source-id=`) {
		t.Fatalf("data-video-source-id stripped: %s", out)
	}
}
```

Run: `go test ./internal/content/ -run 'TestSanitizeHTMLKeeps' -v` → FAIL.

- [ ] **Step 2: Add the attributes**

In `sanitize.go` (~line 46-48), the div attr map:

```go
	"div": {
		"data-type": true, "data-url": true, "data-coupon-id": true,
		"data-display-mode": true,
		// §14 list amendment: internal content embed placeholder target.
		"data-content-slug": true,
		// §106 video: hydrateVideoEmbeds selects .video-embed[data-video-source-id]
		// — without this the attr is stripped and video embeds in rich text
		// bodies (including list items) render as dead placeholders.
		"data-video-source-id": true,
	},
```

NOTE: `data-video-source-id` is NOT currently whitelisted — the VideoNode comment ("the sanitizer strips the data-* attributes… node degrades to an empty .video-embed div") documents it, so TipTap video embeds inside rich-text bodies are dead placeholders today (the `/video/{slug}` page renders from `content.video_source_id`, not hydration). Whitelisting it is required for the list video embed AND repairs the existing dead path. Do NOT whitelist `data-embed-html`/`data-source-type` — hydration only reads the id.

- [ ] **Step 3: Verify + commit**

```bash
go test ./internal/content/ -run TestSanitize -v
git add internal/content/sanitize.go internal/content/sanitize_test.go
git commit -m "Allow data-content-slug on div for list content-embed placeholders"
```

---

### Task 6: Backend — handler-level gate tests

**Files:**
- Test: `internal/content/handlers_test.go` (or wherever `CreateContent`/`UpdateContent` tests live — find the existing story-permission test first: `grep -rn "story.create" internal/content/*_test.go`)

- [ ] **Step 1: Mirror the existing `story.create` test**

Locate a test exercising `CreateContent` with an RBAC resolver (check `internal/content/handlers_autoassign_test.go`, `publish_at_test.go`, `engagement_test.go` for the request/httptest harness — there is no dedicated `story.create` gate test today, so reuse whichever harness builds a `Handlers` + `auth.AuthenticatedUser` context + a stub `rbac` resolver) and cover `list`:
- `content_type=list` without `list.create` → 403 `list_permission_required`.
- `content_type=list` with `list.create` → proceeds past the gate.
- PATCH on a list with oversized `items` → 400 `too_many_items`.

**Caveat — `list.enabled` disabled path:** `h.settings` is a concrete `*config.Settings`; `&config.Settings{}` has nil internals and `GetBoolOr`/`GetIntOr` panic on a zero value (no DB). The handler code guards `h.settings != nil`, so handler tests must construct `Handlers` with **nil** settings — meaning `list_feature_disabled` is NOT unit-testable without a test DB. Cover it in the Task 13 manual smoke step instead; note this in the test file comment.

Run: `go test ./internal/content/ -v` → all PASS.

- [ ] **Step 2: Commit**

```bash
git add internal/content/handlers_test.go
git commit -m "Cover list.create gate, list.enabled killswitch, and item validation in content handler tests"
```

---

### Task 7: Frontend — types, registry, link map, publish gate

**Files:**
- Modify: `apps/main/src/lib/types/index.ts`
- Modify: `apps/main/src/lib/types/content.ts` — add `meta_description?: string | null` to `GuideListItem` (the `ListContentsByTypeRow` already serializes `meta_description`; the interface just doesn't declare it — needed for list-card excerpts)
- Modify: `apps/main/src/lib/components/create/contentTypeRegistry.ts`
- Modify: `apps/main/src/lib/components/create/gate.ts`
- Modify: `apps/main/src/lib/utils/contentLink.ts`
- Test: `apps/main/src/lib/components/create/contentTypeRegistry.test.ts`

**Interfaces:**
- Produces: `FormKind` includes `'list'`; `TypeEntry.requiredPermission?: string`; `createAvailableTypes(types, flags, settings, caller?)` where `caller = { permissions?: string[]; isSuperAdmin?: boolean }`; `contentHref` maps `list` → `/list`; `evaluatePublish` rejects a `list` form with 0 items.

- [ ] **Step 1: Types**

In `apps/main/src/lib/types/index.ts`:
- Find the `ContentType` union (grep `export type ContentType`) — add `'list'`.
- Add:

```ts
export type ListMode = 'numbered' | 'countdown' | 'unnumbered';
export type ListEmbedKind = 'content' | 'url' | 'video';

export interface ListEmbedSnapshot {
	title?: string;
	excerpt?: string;
	image_url?: string;
	content_type?: string;
}

export interface ListEmbedResolved {
	title?: string;
	description?: string;
	image_url?: string;
	site_name?: string;
	embed_url?: string;
}

export interface ListItemEmbed {
	kind: ListEmbedKind;
	content_id?: string;
	content_slug?: string;
	snapshot?: ListEmbedSnapshot;
	url?: string;
	resolved?: ListEmbedResolved;
	video_source_id?: string;
}

export interface ListItem {
	heading?: string;
	body_html?: string;
	media_id?: string | null;
	embed?: ListItemEmbed | null;
	source_url?: string;
	credit?: string;
}

export interface ListFieldValues {
	list_mode: ListMode;
	intro_html?: string;
	items: ListItem[];
}
```

- [ ] **Step 2: Registry**

In `contentTypeRegistry.ts`:
- `export type FormKind = 'standard' | 'comparison' | 'list';`
- `TypeEntry` gets `/** RBAC node required for this type to appear in /create. */ requiredPermission?: string;`
- `COMPOSER_OVERRIDES` add:

```ts
	// §14 list amendment — dedicated item builder; gated by list.create RBAC
	// node and the list.enabled public setting.
	list: {
		formKind: 'list',
		supportsSeries: false,
		featureFlag: 'list.enabled',
		requiredPermission: 'list.create'
	},
```

- `CREATE_ALLOWED_TYPES` add `'list'` (order controls the dropdown; place after `'video'`).
- `createAvailableTypes` signature:

```ts
export function createAvailableTypes(
	types: ContentTypeMeta[],
	flags: Record<string, boolean>,
	settings?: Record<string, unknown>,
	caller?: { permissions?: string[]; isSuperAdmin?: boolean }
): TypeEntry[] {
	const allowed = new Set<string>(CREATE_ALLOWED_TYPES);
	const perms = new Set(caller?.permissions ?? []);
	const canSee = (entry: TypeEntry) =>
		!entry.requiredPermission || caller?.isSuperAdmin === true || perms.has(entry.requiredPermission);
	return availableTypes(types, flags, settings)
		.filter((entry) => allowed.has(entry.value) && canSee(entry))
		.sort((a, b) => (CREATE_TYPE_ORDER[a.value] ?? 0) - (CREATE_TYPE_ORDER[b.value] ?? 0));
}
```

- [ ] **Step 3: contentLink**

In `apps/main/src/lib/utils/contentLink.ts` type map add `list: '/list'`.

- [ ] **Step 4: gate.ts**

- `isExceptionRoute`: add `|| routeId === '/(app)/list/[slug]'` (ADR-058 Exception #1 — CPT singles collapse the left nav).
- `evaluatePublish`: after the comparison branch add (NOTE: do NOT reuse `create.gate.items` — its en.json copy says "Add at least 2 items"; lists require ≥1, so use a distinct key `create.gate.list_items` added in Task 12):

```ts
	if (i.formKind === 'list') {
		if (i.itemCount < 1) return { ok: false, reason: 'create.gate.list_items' };
	}
```

- [ ] **Step 5: Failing tests then verify**

Extend `contentTypeRegistry.test.ts`:
- `resolveType('list')` → `formKind:'list'`, `featureFlag:'list.enabled'`, `requiredPermission:'list.create'`.
- `createAvailableTypes` includes `list` when `permissions:['list.create']`, excludes it when permissions lack the node (and when `isSuperAdmin:true` includes it).
- Excludes `list` when `settings['list.enabled'] === false` even with permission.

Run: `cd apps/main && bunx vitest run src/lib/components/create/contentTypeRegistry.test.ts` (check `package.json` scripts for the actual test command first — likely `bun run test` or `bunx vitest run`).

- [ ] **Step 6: Commit**

```bash
git add apps/main/src/lib/types/index.ts apps/main/src/lib/types/content.ts apps/main/src/lib/components/create/contentTypeRegistry.ts apps/main/src/lib/components/create/gate.ts apps/main/src/lib/utils/contentLink.ts apps/main/src/lib/components/create/contentTypeRegistry.test.ts
git commit -m "Register list composer metadata, route map, and publish gate"
```

---

### Task 8: Frontend — create page permission gate + CreateShell wiring

**Files:**
- Modify: `apps/main/src/routes/(app)/studio/create/+page.server.ts`
- Modify: `apps/main/src/lib/components/create/CreateShell.svelte`

**Interfaces:**
- `createAvailableTypes(rawTypes, flags, siteSettings, callerPerms)` called with the session user's permission nodes.
- `ListBuilder` props (defined fully in Task 9): `initial: ListFieldValues | null`, `onvalues: (fv: ListFieldValues) => void`, `onhtml: (html: string) => void`, `oncounts: (n: number) => void`, `onedit: () => void`, `defaultEditor`.

- [ ] **Step 1: Page server**

In `studio/create/+page.server.ts` line ~276:

```ts
const callerPerms = {
	permissions: event.locals.user?.permissions ?? [],
	isSuperAdmin: (event.locals.user?.groups ?? []).some(
		(g) => g === 'super_admin' || g === 'Super Admin'
	)
};
```
then `types: createAvailableTypes(rawTypes, flags, siteSettings, callerPerms)`.

- [ ] **Step 2: CreateShell — list state + snapshot**

In `CreateShell.svelte`:
- `const isList = $derived(selectedType.value === 'list');`
- `let listFieldValues = $state<ListFieldValues>(untrack(() => draft?.content_type === 'list' ? ((draft.field_values as unknown as ListFieldValues | null) ?? { list_mode: 'numbered', items: [] }) : { list_mode: 'numbered', items: [] }));` — `field_values` is `Record<string, unknown> | null`; the double cast is required because interfaces have no index signature (plain `as` fails typecheck).
- `let listItemCount = $state(0);` — seeded from `listFieldValues.items.length`.
- In the `snapshot` `$derived`, `field_values` gains: `...(isList ? listFieldValues : {})` (same spread mechanism as the guide dimension/purpose keys at line ~240).
- `gate`: `itemCount: isList ? listItemCount : items.length`.
- In the editor branch (~line 517), add `{:else if selectedType.formKind === 'list'}` BEFORE the `{:else}` PostEditor:

```svelte
		{:else if selectedType.formKind === 'list'}
			<ListBuilder
				initial={draft?.content_type === 'list' ? (draft.field_values as unknown as ListFieldValues) : null}
				defaultEditor={defaultEditor}
				onvalues={(fv) => { listFieldValues = fv; }}
				onhtml={(v) => { bodyHtml = v; }}
				oncounts={(n) => { listItemCount = n; }}
				onedit={markDirty}
			/>
```

- Import `ListBuilder` from `./editors/ListBuilder.svelte` and `ListFieldValues` from `$lib/types`.

Note: `activeFieldValues` filters `fieldValues` to custom-field-group keys; `list_*`/`items`/`intro_html` bypass it via the direct spread — same mechanism `dimension`/`purpose` already use.

- [ ] **Step 3: Commit** (ListBuilder lands in Task 9 — commit Task 8's page-server change separately to keep the tree compiling: the CreateShell edit depends on ListBuilder existing, so stage CreateShell + page-server + Task 9 files together.)

```bash
git add "apps/main/src/routes/(app)/studio/create/+page.server.ts"
git commit -m "Pass caller permissions to the create type selector (list.create gate)"
```

---

### Task 9: Frontend — `listSerialize.ts` + `ListBuilder.svelte`

**Files:**
- Create: `apps/main/src/lib/components/create/editors/listSerialize.ts` (pure serialization — testable seam)
- Create: `apps/main/src/lib/components/create/editors/ListBuilder.svelte`
- Test: `apps/main/src/lib/components/create/editors/listSerialize.test.ts`

**Interfaces:**
- `serializeListBodyHtml(mode: ListMode, introHtml: string, items: ListItem[], mediaUrls: Record<string,string>): string` — full body_html.
- `serializeListBody(mode, introHtml, items): string` — not needed: `body` derives server-side from `body_html` via `SanitizeText`. Only emit `body_html`.
- `displayNumber(mode, index, total): number` — numbered/unnumbered: `index+1`; countdown: `total - index`.

- [ ] **Step 1: Failing serialization tests**

`listSerialize.test.ts` cases:
- `numbered` mode with 3 items → `<span class="li-num">1.</span>` … `3.</span>`, `id="item-1..3"` in order.
- `countdown` mode with 3 items → first emitted item carries `li-num 3`, last `1`.
- `unnumbered` → no `li-num` spans.
- `embed.kind='content'` → `<div data-type="content-embed" data-content-slug="X">` with the snapshot card markup inside (title + excerpt + image `<a>` link).
- `embed.kind='url'` → `<div data-type="link-preview" data-url="…">`; when `resolved.embed_url` present → `<iframe src="…">` instead.
- `embed.kind='video'` → `<div class="video-embed" data-video-source-id="…">` with thumbnail `<img>` child.
- `media_id` → `<img src="{mediaUrls[id]}" alt="">` inside the item (`loading` is not whitelisted — it is stripped on save; don't emit it).
- `source_url`/`credit` → `<p class="li-credit">` with `<a href>` when source_url set.
- HTML escaping: heading text is escaped (`&<>`), item `body_html` is embedded raw (it is sanitized server-side).
- `intro_html` precedes the first `.list-item`.

Run `bunx vitest run src/lib/components/create/editors/listSerialize.test.ts` → FAIL (module missing).

- [ ] **Step 2: Implement `listSerialize.ts`**

```ts
import type { ListItem, ListMode } from '$lib/types';

const esc = (s: string) =>
	s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

export function displayNumber(mode: ListMode, index: number, total: number): number {
	return mode === 'countdown' ? total - index : index + 1;
}

function embedHtml(item: ListItem, mediaUrls: Record<string, string>): string {
	if (item.media_id && mediaUrls[item.media_id]) {
		// NOTE: `loading` is not in the img attr whitelist (sanitize.go) — it
		// would be stripped on save. Only src/alt/title/width/height survive.
		return `<img src="${esc(mediaUrls[item.media_id])}" alt="">`;
	}
	const e = item.embed;
	if (!e) return '';
	if (e.kind === 'content' && e.content_slug) {
		const snap = e.snapshot ?? {};
		const card = `<a href="/content/${esc(e.content_slug)}" class="ce-card">` +
			(snap.image_url ? `<img src="${esc(snap.image_url)}" alt="">` : '') +
			`<span class="ce-title">${esc(snap.title ?? e.content_slug)}</span>` +
			(snap.excerpt ? `<span class="ce-excerpt">${esc(snap.excerpt)}</span>` : '') +
			`</a>`;
		return `<div data-type="content-embed" data-content-slug="${esc(e.content_slug)}">${card}</div>`;
	}
	if (e.kind === 'url' && e.url) {
		if (e.resolved?.embed_url) {
			return `<iframe src="${esc(e.resolved.embed_url)}" allowfullscreen loading="lazy"></iframe>`;
		}
		return `<div data-type="link-preview" data-url="${esc(e.url)}"></div>`;
	}
	if (e.kind === 'video' && e.video_source_id) {
		// Poster child gives a no-JS preview; hydrateVideoEmbeds replaces
		// children with the player. Thumbnail rides in resolved.image_url.
		const poster = e.resolved?.image_url
			? `<img src="${esc(e.resolved.image_url)}" alt="" class="video-embed-thumb">`
			: '';
		return `<div class="video-embed" data-video-source-id="${esc(e.video_source_id)}">${poster}</div>`;
	}
	return '';
}

export function serializeListBodyHtml(
	mode: ListMode,
	introHtml: string,
	items: ListItem[],
	mediaUrls: Record<string, string>
): string {
	const parts: string[] = [];
	if (introHtml.trim()) parts.push(introHtml);
	items.forEach((item, i) => {
		const n = i + 1;
		const num = displayNumber(mode, i, items.length);
		const badge = mode === 'unnumbered' ? '' : `<span class="li-num">${num}.</span> `;
		const heading = item.heading?.trim()
			? `<h3>${badge}${esc(item.heading.trim())}</h3>`
			: badge ? `<h3>${badge.trim()}</h3>` : '';
		const body = item.body_html?.trim() ? item.body_html : '';
		const visual = embedHtml(item, mediaUrls);
		const credit =
			item.source_url || item.credit
				? `<p class="li-credit">${item.source_url ? `<a href="${esc(item.source_url)}">${esc(item.credit || item.source_url)}</a>` : esc(item.credit ?? '')}</p>`
				: '';
		parts.push(`<div class="list-item" id="item-${n}">${heading}${visual}${body}${credit}</div>`);
	});
	return parts.join('');
}
```

Refine during implementation to match the actual sanitize whitelist (check which attributes `iframe` accepts — `src`, `allowfullscreen`, `loading` are in sanitize.go's iframe handling; confirm before relying on `loading`).

- [ ] **Step 3: Implement `ListBuilder.svelte`**

Component shape (Svelte 5 runes, match sibling editors' style — look at `ComparisonBuilder.svelte` for conventions):

```svelte
<script lang="ts">
	// Ref: docs/superpowers/specs/2026-09-28-list-content-type-design.md (§14 list)
	import type { ListFieldValues, ListItem, ListMode } from '$lib/types';
	import PostEditor from './PostEditor.svelte';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import Autocomplete, { type AutocompleteItem } from '../Autocomplete.svelte';
	import { api } from '$lib/api/client';
	import { resolveVideo } from '$lib/api/videos';
	import { serializeListBodyHtml } from './listSerialize';
	import { t } from '$lib/i18n';

	interface Props {
		initial: ListFieldValues | null;
		defaultEditor: string;
		onvalues: (fv: ListFieldValues) => void;
		onhtml: (html: string) => void;
		oncounts: (n: number) => void;
		onedit: () => void;
	}
	let { initial, defaultEditor, onvalues, onhtml, oncounts, onedit }: Props = $props();

	let mode = $state<ListMode>(initial?.list_mode ?? 'numbered');
	let introHtml = $state(initial?.intro_html ?? '');
	let items = $state<ListItem[]>(initial?.items ?? []);
	let mediaUrls = $state<Record<string, string>>({});
	// resolve media URLs for restored items
	// (on mount: for each item.media_id → api.get(`/media/${id}`) → mediaUrls[id]=url)
</script>
```

UI structure:
- Intro `PostEditor` bound to `introHtml` (`onhtml={(v)=>{introHtml=v; emit()}}`).
- Mode segmented control (three buttons or `<select>` — check existing UI primitives under `$lib/components/ui`).
- `{#each items as item, i (itemKey(item))}` item card: position badge showing `displayNumber(mode,i,items.length)`; heading `<input>`; per-item body editor (reuse `PostEditor` without `aiTrigger` — its Props are `{html, onedit, onhtml, onjson?, onfocus?, defaultMode?, aiTrigger?}`); **one visual slot** tab control: `Image | JapaConnect | Link | Video`.
  - Image tab → opens `MediaLibraryModal` (`onSelect={(m)=>{ item.media_id=m.id; mediaUrls[m.id]=m.url; item.embed=null }}`); when an embed exists it is cleared (XOR).
  - JapaConnect tab → `Autocomplete` (`fetcher: (q: string) => Promise<AutocompleteItem[]>`) with fetcher `async (q) => { const res = await api.get<GuideListItem[]>(`/content?q=${encodeURIComponent(q)}&page_size=10`); return (res.data ?? []).map((c) => ({ id: c.id, label: c.title })); }` — **do not pass `type=`** (in `ListContent`'s branch order the `type` branch swallows `q`; search must go out bare). Keep the picked row so `onSelect` can `api.get(`/content/${row.slug}`)` → snapshot `{title, excerpt, image_url: dto.featured_image_url, content_type}` and store `embed={kind:'content', content_id: row.id, content_slug: row.slug, snapshot}`.
  - Link tab → URL input + "Fetch" button → `api.get('/link-preview?url='+encodeURIComponent(url))` → response `{data:{url,title,description,image,site_name,favicon}}` → store `embed={kind:'url', url, resolved:{title, description, image_url: preview.image, site_name}}` (map `image`→`image_url`; no `embed_url` is returned — see reconciliation 4).
  - Video tab → URL input → `resolveVideo(url)` from `$lib/api/videos` → `{data:{source, embed_json}}` → `embed={kind:'video', video_source_id: res.data.source.id, resolved: {image_url: res.data.source.thumbnail_url ?? undefined}}` — the thumbnail lands in `resolved.image_url` and the serializer emits it as the poster `<img>` child. Requires `video.import.single` — on 403 show the i18n'd `create.list.embed.video_denied` string. `POST /videos/resolve` is auth-gated, which is fine (composer is authed).
- Per item: `source_url` input, `credit` input, remove button, up/down reorder buttons (minimum per spec; drag is optional).
- "Add item" button (appends empty item).
- `emit(markDirty = true)` — calls `onvalues({list_mode: mode, intro_html: introHtml, items})`, `onhtml(serializeListBodyHtml(mode, introHtml, items, mediaUrls))`, `oncounts(items.length)`, and `onedit()` only when `markDirty` is true. **Mount-time emit passes `markDirty=false`** — otherwise restoring a draft immediately schedules an autosave PATCH identical to stored data (noisy revision).

Give each `ListItem` a stable client-side key for the keyed each — keep `items` elements as `{key: string, item: ListItem}` internally and map to `ListItem[]` on emit so no `_key` leaks into `field_values`.

- [ ] **Step 4: Wire check**

`bun run check` (svelte-check) in `apps/main` → clean. Then commit Tasks 8+9 together:

```bash
git add apps/main/src/lib/components/create/CreateShell.svelte apps/main/src/lib/components/create/editors/ListBuilder.svelte apps/main/src/lib/components/create/editors/listSerialize.ts apps/main/src/lib/components/create/editors/listSerialize.test.ts
git commit -m "Add ListBuilder composer editor for the list content type"
```

---

### Task 10: Frontend — content-embed hydration

**Files:**
- Create: `apps/main/src/lib/tiptap/hydrateContentEmbeds.ts`
- Create: `apps/main/src/lib/components/content/ContentEmbedCard.svelte`
- Modify: `apps/main/src/lib/components/content/ContentArticlePage.svelte`
- Test: `apps/main/src/lib/tiptap/hydrateContentEmbeds.test.ts` (if the repo has action tests — check `ls apps/main/src/lib/tiptap/*.test.ts`)

**Interfaces:**
- `hydrateContentEmbeds(node: HTMLElement): { destroy(): void }` — Svelte action, same shape as `hydrateVideoEmbeds`.
- `ContentEmbedCard` props: `{ slug: string; initial?: ListEmbedSnapshot }`.

- [ ] **Step 1: `ContentEmbedCard.svelte`**

Props `{ slug, initial }`. On mount fetch `api.get(`/content/${slug}`)` → render card: image (`featured_image_url`), `content_type` badge, title linking to `contentHref(dto)`, excerpt, author name/avatar. On 404/error → render `initial` snapshot if present, else an i18n'd "content unavailable" state. Style to match `LinkPreviewCard.svelte` (read it and mirror class conventions).

- [ ] **Step 2: `hydrateContentEmbeds.ts`**

Mirror `hydrateVideoEmbeds.ts` exactly (MutationObserver + `data-hydrated` guard + mount/unmount tracking):

```ts
import { mount, unmount } from 'svelte';
import ContentEmbedCard from '$lib/components/content/ContentEmbedCard.svelte';

export function hydrateContentEmbeds(node: HTMLElement) {
	const mounted = new Map<HTMLElement, Record<string, unknown>>();
	function scan() {
		for (const el of node.querySelectorAll<HTMLElement>(
			'div[data-type="content-embed"][data-content-slug]:not([data-hydrated])'
		)) {
			const slug = el.dataset.contentSlug;
			if (!slug) continue;
			el.setAttribute('data-hydrated', 'true');
			const comp = mount(ContentEmbedCard, { target: el, props: { slug } });
			mounted.set(el, comp);
		}
	}
	scan();
	const observer = new MutationObserver(() => scan());
	observer.observe(node, { childList: true, subtree: true });
	return {
		destroy() {
			observer.disconnect();
			for (const [, c] of mounted) unmount(c);
			mounted.clear();
		}
	};
}
```

- [ ] **Step 3: Wire into `ContentArticlePage.svelte`**

Add the import and `use:hydrateContentEmbeds` on the prose container at line ~358 (alongside `hydrateLinkPreviews`/`hydrateMentions`/`hydrateVideoEmbeds`).

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/lib/tiptap/hydrateContentEmbeds.ts apps/main/src/lib/components/content/ContentEmbedCard.svelte apps/main/src/lib/components/content/ContentArticlePage.svelte
git commit -m "Hydrate list content-embed placeholders into live content cards"
```

---

### Task 11: Frontend — `/list/[slug]` detail + `/lists` index

**Files:**
- Create: `apps/main/src/routes/(app)/list/[slug]/+page.server.ts`
- Create: `apps/main/src/routes/(app)/list/[slug]/+page.svelte`
- Create: `apps/main/src/routes/(app)/lists/+page.server.ts`
- Create: `apps/main/src/routes/(app)/lists/+page.svelte`

- [ ] **Step 1: Detail route**

`+page.server.ts` — mirror `guide/[slug]/+page.server.ts` minus the scholarship block, plus the settings gate:

```ts
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { loadContentArticle } from '$lib/server/content-article';
import { contentHref } from '$lib/utils/contentLink';

export const load: PageServerLoad = async (event) => {
	const parent = await event.parent();
	const siteSettings = (parent as { siteSettings?: Record<string, unknown> }).siteSettings ?? {};
	if (siteSettings['list.enabled'] === false) {
		throw error(404, { code: 'feature_disabled', message: 'Lists are not available' });
	}
	const data = await loadContentArticle(event);
	if (data.article.content_type !== 'list') {
		throw redirect(301, `${contentHref(data.article)}${event.url.search}`);
	}
	return data;
};
```

`+page.svelte` — `<ContentArticlePage {data} />` (identical shape to the guide page; hydration actions already live inside the component).

`/content/[slug]` needs no change — `contentHref` now returns `/list/{slug}` for list items so the generic route 301s automatically.

- [ ] **Step 2: Index route — model on `/guides` exactly**

`GET /content?type=list` returns `{data: GuideListItem[]}` with **no `meta.total`** — `ListContentsByTypeRow` serializes directly (title/slug/body/meta_description/author_username/author_display_name/space_name/counts/`featured_image_id` — but NOT `featured_image_url`/`header_image_url`, which are detail-DTO-only; cards render without covers, same as `/guides`). Pagination: composite cursor `created_at|id` for `latest`, `page_offset` for `popular` (view_count order has no cursor), single page for `featured`.

`+page.server.ts` — copy `guides/+page.server.ts` structure minus categories/series/roadmap:

```ts
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGetEnvelope } from '$lib/server/api';
import { attachAuthorUsernames } from '$lib/server/users';
import type { GuideListItem } from '$lib/types/content';

// Ref: japaconnect-engineering/sections/14-content-type-system.md (List amendment)
const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	// list.enabled is a PUBLIC system setting (video.enabled pattern) — the
	// root layout exposes it via siteSettings. Missing = enabled.
	const parent = await event.parent();
	const siteSettings = (parent as { siteSettings?: Record<string, unknown> }).siteSettings ?? {};
	if (siteSettings['list.enabled'] === false) {
		throw error(404, { code: 'feature_disabled', message: 'Lists are not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const tab = event.url.searchParams.get('tab') ?? 'latest'; // 'latest' | 'popular' | 'featured'
	const cursor = event.url.searchParams.get('cursor') ?? '';
	const offset = parseInt(event.url.searchParams.get('offset') ?? '0', 10) || 0;

	const params = new URLSearchParams();
	params.set('type', 'list');
	params.set('page_size', String(PAGE_SIZE));
	if (tab === 'featured') {
		params.set('featured', '1');
	} else if (tab === 'popular') {
		params.set('sort', 'popular');
		if (offset > 0) params.set('page_offset', String(offset));
	} else if (cursor) {
		params.set('cursor', cursor);
	}

	let items: GuideListItem[] = [];
	let loadError = false;
	try {
		const res = await apiGetEnvelope<GuideListItem[]>(ctx, `/content?${params.toString()}`);
		items = res.data ?? [];
		await attachAuthorUsernames(ctx, items);
	} catch {
		loadError = true;
	}

	const last = items[items.length - 1];
	const nextCursor =
		tab === 'popular'
			? items.length >= PAGE_SIZE ? `offset:${offset + items.length}` : null
			: tab === 'latest' && items.length >= PAGE_SIZE && last
				? `${last.created_at}|${last.id}`
				: null;

	return { items, tab, authed, pageSize: PAGE_SIZE, nextCursor, loadError };
};
```

`+page.svelte` — copy `guides/+page.svelte` structure minus the category Autocomplete/series/roadmap panels: `ListPageBar` header, `UrlTabs` (`latest`/`popular`/`featured`), `items.map(toFeedItem)` → `ContentGridCard` grid (`toFeedItem` with `content_type: 'list'`, `excerpt: g.meta_description ?? undefined` — check `GuideListItem` exposes `meta_description`; if not, leave `excerpt` undefined like guides' `purpose` usage), "Load more" button navigating with `?cursor=`/`?offset=` per `nextCursor` (the `offset:N` pseudo-cursor convention the guides page already parses). Empty → `EmptyState`; failure → `loadError` state. All chrome strings via `t('lists.*')`.

- [ ] **Step 3: Verify**

```bash
cd apps/main && bun run check
```
Expected: svelte-check clean for the four new files.

- [ ] **Step 4: Commit**

```bash
git add "apps/main/src/routes/(app)/list" "apps/main/src/routes/(app)/lists"
git commit -m "Add /list/{slug} detail and /lists index routes"
```

---

### Task 12: i18n

**Files:**
- Modify: `apps/main/src/lib/i18n/en.json`
- Modify: `apps/main/src/lib/i18n/fr.json`

- [ ] **Step 1: Add keys (en + fr)**

Add under the `create`/`lists` namespaces (check the file's structure — flat keys like `"create.gate.items"`):

```
create.types.list            "List" / "Liste"
create.list.mode             "List mode" / "Mode de liste"
create.list.mode.numbered    "Numbered" / "Numérotée"
create.list.mode.countdown   "Countdown" / "Compte à rebours"
create.list.mode.unnumbered  "Unnumbered" / "Non numérotée"
create.list.intro_label      "Introduction" / "Introduction"
create.list.add_item         "Add item" / "Ajouter un élément"
create.list.item_heading     "Item heading (optional)" / "Titre de l'élément (facultatif)"
create.list.item_body        "Item body" / "Texte de l'élément"
create.list.embed.image      "Image" / "Image"
create.list.embed.content    "JapaConnect" / "JapaConnect"
create.list.embed.link       "External link" / "Lien externe"
create.list.embed.video      "Video" / "Vidéo"
create.list.embed.search     "Search content…" / "Rechercher du contenu…"
create.list.embed.fetch      "Fetch" / "Récupérer"
create.list.embed.video_denied "You don't have permission to resolve videos" / "Vous n'avez pas la permission de résoudre des vidéos"
create.list.source_url       "Source URL" / "URL source"
create.list.credit           "Credit" / "Crédit"
create.list.remove_item      "Remove item" / "Supprimer l'élément"
create.gate.list_items       "Add at least 1 item" / "Ajoutez au moins 1 élément"
lists.title                  "Lists" / "Listes"
lists.description            "Ranked and curated lists from the community" / "Listes classées et sélectionnées par la communauté"
lists.empty                  "No lists yet" / "Aucune liste pour le moment"
lists.sort.latest            "Latest" / "Récentes"
lists.sort.popular           "Popular" / "Populaires"
lists.tab.featured           "Featured" / "À la une"
content.embed.unavailable    "This content is no longer available" / "Ce contenu n'est plus disponible"
```

Verify key names against the actual flat/nested JSON structure of en.json before writing.

- [ ] **Step 2: Commit**

```bash
git add apps/main/src/lib/i18n/en.json apps/main/src/lib/i18n/fr.json
git commit -m "Add list builder and lists index i18n strings (en/fr)"
```

---

### Task 13: Final verification + docs touch-up

- [ ] **Step 1: Backend**

```bash
go build ./... && go test ./internal/content/ ./internal/control/ -v
```
Expected: build clean, all tests pass.

- [ ] **Step 2: Frontend**

```bash
cd apps/main && bun run check && bunx vitest run src/lib/components/create src/lib/tiptap
```
Expected: typecheck clean, tests pass.

- [ ] **Step 3: Migration down/up round-trip**

The migrator is forward-only — `.down.sql` files are never run by `ListMigrationFiles` (AUDIT-BE-003) and there is no `migrate-down` Make target. Rollback is manual via `psql`; to round-trip, run the down file, clear the tracking row, then re-apply:

```bash
psql $JC_DATABASE_URL -f migrations/000749_list_content_type.down.sql
psql $JC_DATABASE_URL -c "DELETE FROM schema_migrations WHERE filename = '000749_list_content_type.up.sql';"
psql $JC_DATABASE_URL -c "SELECT key FROM permissions WHERE key = 'list.create';"  # expect: 0 rows
make migrate
psql $JC_DATABASE_URL -c "SELECT key FROM permissions WHERE key = 'list.create';"  # expect: 1 row
```
Expected: down removes the seeds cleanly; up re-seeds `list.create`/`agent.content_type.list`/`ItemList` template.

- [ ] **Step 4: Manual smoke (if a dev environment is available)**

1. Admin: grant `list.create` to a test group via RBAC UI.
2. Member in group → `/studio/create` shows "List"; member outside → hidden.
3. Compose a 3-item countdown list with one internal embed + one external link + one image → publish.
4. `/list/{slug}` renders numbered badges 3→1, embeds hydrate, `/lists` lists it, `/content/{slug}` 301s, ItemList JSON-LD present in page data.
5. Toggle `list.enabled=false` → composer hides the type, `/lists` 404s, `POST /content` with `content_type=list` → 403. Re-enable.

- [ ] **Step 5: Final commit** for any straggler files.

---

## Notes for the implementer

- `agent.content_type.*` needs no Go code — `allowedContentTypes` derives the node name from the type catalog (`internal/ai/tools_content.go:66`); once `TypeList` is registered and the permission key is seeded, agent gating works.
- `ContentArticlePage` renders `body_html` verbatim — List items render correctly with zero renderer changes; hydration is additive.
- `body` plain-text derives server-side (`SanitizeText(body_html)`, handlers.go ~936-939) so item text flows into `content_hash`, `read_time_minutes`, and Meilisearch with no extra work.
- `content_revisions` snapshots `field_values` on every save — item edit history is free.
- The `/lists` nav menu item (if admins add one) is NOT flag-hidden — `routeFeatureFlags` only reads the `features` map and `list.enabled` is a public setting, not a feature flag. Documented limitation matching the `video.enabled` precedent.
- **Sitemap needs no change** — `internal/seo/sitemap.go` emits `/content/{slug}` for every content type (guides, stories, videos included); those URLs 301 to the canonical route. Lists inherit the same behavior — do not "fix" it per-type in this feature.
- `img` attrs that survive the sanitizer are only `src/alt/title/width/height` — do not emit `loading`/`class`-only decorators expecting them to persist. `iframe` has NO whitelist entry at all (even `src` is stripped), which is why the `resolved.embed_url` iframe path stays dormant.
