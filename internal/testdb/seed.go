//go:build integration

package testdb

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// UserOpts customizes SeedUser. Zero-value fields fall back to generated
// or default values: Username to a random unique handle and Status to
// "active". DisplayName is optional — when set, SeedUser also inserts a
// matching user_profiles row (the table the public profile query LEFT
// JOINs for display_name/avatar_url/bio).
type UserOpts struct {
	Username    string
	DisplayName string
	Status      string
}

// SeedUser inserts a user row — and, when o.DisplayName is set, a matching
// user_profiles row — under tenantID and returns the new user id.
func SeedUser(t *testing.T, d *db.DB, tenantID pgtype.UUID, o UserOpts) pgtype.UUID {
	t.Helper()

	ctx := t.Context()
	userID := mustUUID()

	username := o.Username
	if username == "" {
		username = "user-" + uuid.New().String()
	}

	status := o.Status
	if status == "" {
		status = "active"
	}

	email := username + "@example.test"

	_, err := d.Writer.Exec(ctx,
		`INSERT INTO users (id, tenant_id, email, username, status) VALUES ($1, $2, $3, $4, $5)`,
		userID, tenantID, email, username, status)
	if err != nil {
		t.Fatalf("testdb: SeedUser: inserting user: %v", err)
	}

	if o.DisplayName != "" {
		_, err := d.Writer.Exec(ctx,
			`INSERT INTO user_profiles (tenant_id, user_id, display_name) VALUES ($1, $2, $3)`,
			tenantID, userID, o.DisplayName)
		if err != nil {
			t.Fatalf("testdb: SeedUser: inserting user_profiles: %v", err)
		}
	}

	return userID
}

// SeedFollowers creates n throwaway follower users and a follows row from
// each to entityUserID, then recomputes entityUserID's denormalized
// follower_count from the resulting rows.
//
// follower_count is NOT trigger-maintained: migrations/000012_follow_system.sql
// documents it as "maintained in the service layer (not via triggers)
// because not all entity tables exist yet," and no later forward migration
// adds a trigger (verified by grepping every migrations/*.sql file for
// CREATE TRIGGER — the only hit, 000017_forum_qa.sql, is an unrelated
// reply-view-count trigger). The harness must therefore set the column
// explicitly here, or any test asserting follower_count after SeedFollowers
// would see 0 regardless of how many follows rows were inserted.
func SeedFollowers(t *testing.T, d *db.DB, tenantID, entityUserID pgtype.UUID, n int) {
	t.Helper()

	ctx := t.Context()

	for range n {
		followerID := SeedUser(t, d, tenantID, UserOpts{Username: "follower-" + uuid.New().String()})

		_, err := d.Writer.Exec(ctx,
			`INSERT INTO follows (tenant_id, follower_id, entity_type, entity_id) VALUES ($1, $2, 'user', $3)`,
			tenantID, followerID, entityUserID)
		if err != nil {
			t.Fatalf("testdb: SeedFollowers: inserting follow: %v", err)
		}
	}

	_, err := d.Writer.Exec(ctx,
		`UPDATE users SET follower_count = (
			SELECT count(*) FROM follows
			WHERE tenant_id = $1 AND entity_type = 'user' AND entity_id = $2
		) WHERE id = $2 AND tenant_id = $1`,
		tenantID, entityUserID)
	if err != nil {
		t.Fatalf("testdb: SeedFollowers: updating follower_count: %v", err)
	}
}

// SeedFollowing creates n throwaway followee users and a follows row from
// followerID to each. There is no "following" counter column on users
// (only follower_count is denormalized), so no count update is needed.
func SeedFollowing(t *testing.T, d *db.DB, tenantID, followerID pgtype.UUID, n int) {
	t.Helper()

	ctx := t.Context()

	for range n {
		followeeID := SeedUser(t, d, tenantID, UserOpts{Username: "followee-" + uuid.New().String()})

		_, err := d.Writer.Exec(ctx,
			`INSERT INTO follows (tenant_id, follower_id, entity_type, entity_id) VALUES ($1, $2, 'user', $3)`,
			tenantID, followerID, followeeID)
		if err != nil {
			t.Fatalf("testdb: SeedFollowing: inserting follow: %v", err)
		}
	}
}

// ContentOpts customizes SeedContent. Zero-value fields default to a
// generated Title, Status "published", and Visibility "public" — the
// platform's public-visibility combination, so seeded content is visible
// to public profile reads unless a test deliberately opts out.
// FeaturedImageID is optional; a zero pgtype.UUID{} inserts NULL.
type ContentOpts struct {
	Title           string
	Status          string
	Visibility      string
	FeaturedImageID pgtype.UUID
	OrganizationID  pgtype.UUID
	SpaceID         pgtype.UUID
}

// SeedContent inserts a contents row authored by authorID under tenantID
// and returns its id.
func SeedContent(t *testing.T, d *db.DB, tenantID, authorID pgtype.UUID, o ContentOpts) pgtype.UUID {
	t.Helper()

	ctx := t.Context()
	contentID := mustUUID()

	title := o.Title
	if title == "" {
		title = "Test Content " + uuid.New().String()
	}

	status := o.Status
	if status == "" {
		status = "published"
	}

	visibility := o.Visibility
	if visibility == "" {
		visibility = "public"
	}

	slug := "test-content-" + uuid.New().String()

	_, err := d.Writer.Exec(ctx,
		`INSERT INTO contents (id, tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html, status, visibility, featured_image_id)
		 VALUES ($1, $2, $3, $4, $5, 'discussion', $6, $7, 'Seeded test body.', '<p>Seeded test body.</p>', $8, $9, $10)`,
		contentID, tenantID, authorID, o.OrganizationID, o.SpaceID, title, slug, status, visibility, o.FeaturedImageID)
	if err != nil {
		t.Fatalf("testdb: SeedContent: %v", err)
	}

	return contentID
}

// SeedComment inserts a comment authored by authorID on contentID under
// tenantID and returns its id. An empty body falls back to a generated
// placeholder.
func SeedComment(t *testing.T, d *db.DB, tenantID, authorID, contentID pgtype.UUID, body string) pgtype.UUID {
	t.Helper()

	ctx := t.Context()
	commentID := mustUUID()

	if body == "" {
		body = "Seeded test comment " + uuid.New().String()
	}

	// Wrap the plain-text body in a minimal TipTap JSON document envelope
	// (Section 49 — comments.body is jsonb TipTap JSON).
	doc := fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":%s}]}]}`,
		strconv.Quote(body))

	_, err := d.Writer.Exec(ctx,
		`INSERT INTO comments (id, tenant_id, content_id, author_user_id, body) VALUES ($1, $2, $3, $4, $5)`,
		commentID, tenantID, contentID, authorID, doc)
	if err != nil {
		t.Fatalf("testdb: SeedComment: %v", err)
	}

	return commentID
}

// SeedReaction records userID's reaction_key reaction on (entityType,
// entityID) under tenantID. reactions.reaction_key has a composite foreign
// key to reaction_types(tenant_id, key), so SeedReaction first upserts a
// reaction_types row for the given key — a fresh tenant from NewTenant has
// none of the default-tenant's seeded reaction types.
func SeedReaction(t *testing.T, d *db.DB, tenantID, userID pgtype.UUID, entityType string, entityID pgtype.UUID, key string) {
	t.Helper()

	ctx := t.Context()

	_, err := d.Writer.Exec(ctx,
		`INSERT INTO reaction_types (tenant_id, key, emoji, label)
		 VALUES ($1, $2, '👍', $3)
		 ON CONFLICT (tenant_id, key) DO NOTHING`,
		tenantID, key, key)
	if err != nil {
		t.Fatalf("testdb: SeedReaction: seeding reaction_types: %v", err)
	}

	_, err = d.Writer.Exec(ctx,
		`INSERT INTO reactions (tenant_id, entity_type, entity_id, user_id, reaction_key) VALUES ($1, $2, $3, $4, $5)`,
		tenantID, entityType, entityID, userID, key)
	if err != nil {
		t.Fatalf("testdb: SeedReaction: %v", err)
	}
}

// SeedMedia inserts a confirmed media row uploaded by uploaderID under
// tenantID and returns its id.
func SeedMedia(t *testing.T, d *db.DB, tenantID, uploaderID pgtype.UUID) pgtype.UUID {
	t.Helper()

	ctx := t.Context()
	mediaID := mustUUID()
	storagePath := "test/" + uuid.New().String() + ".jpg"

	_, err := d.Writer.Exec(ctx,
		`INSERT INTO media (id, tenant_id, uploader_user_id, original_filename, mime_type, file_size_bytes, storage_path, confirmed)
		 VALUES ($1, $2, $3, 'test.jpg', 'image/jpeg', 1024, $4, true)`,
		mediaID, tenantID, uploaderID, storagePath)
	if err != nil {
		t.Fatalf("testdb: SeedMedia: %v", err)
	}

	return mediaID
}

// SpaceOpts customizes SeedSpaceMembership. Zero-value fields fall back to
// defaults: Visibility to "public" and Status to "active" — matching the
// spaces table's own column defaults.
type SpaceOpts struct {
	Visibility string // default "public"
	Status     string // default "active"
}

// SeedSpaceMembership creates a throwaway space (and the space_type row it
// requires) under tenantID and adds userID as a member, returning the new
// space id.
func SeedSpaceMembership(t *testing.T, d *db.DB, tenantID, userID pgtype.UUID, o SpaceOpts) pgtype.UUID {
	t.Helper()

	ctx := t.Context()

	visibility := o.Visibility
	if visibility == "" {
		visibility = "public"
	}

	status := o.Status
	if status == "" {
		status = "active"
	}

	spaceTypeID := mustUUID()
	typeSlug := "test-space-type-" + uuid.New().String()

	_, err := d.Writer.Exec(ctx,
		`INSERT INTO space_types (id, tenant_id, name, slug) VALUES ($1, $2, 'Test Space Type', $3)`,
		spaceTypeID, tenantID, typeSlug)
	if err != nil {
		t.Fatalf("testdb: SeedSpaceMembership: seeding space_types: %v", err)
	}

	spaceID := mustUUID()
	spaceSlug := "test-space-" + uuid.New().String()

	_, err = d.Writer.Exec(ctx,
		`INSERT INTO spaces (id, tenant_id, space_type_id, name, slug, level, visibility, status) VALUES ($1, $2, $3, 'Test Space', $4, 'custom', $5, $6)`,
		spaceID, tenantID, spaceTypeID, spaceSlug, visibility, status)
	if err != nil {
		t.Fatalf("testdb: SeedSpaceMembership: seeding spaces: %v", err)
	}

	_, err = d.Writer.Exec(ctx,
		`INSERT INTO space_members (space_id, user_id, tenant_id) VALUES ($1, $2, $3)`,
		spaceID, userID, tenantID)
	if err != nil {
		t.Fatalf("testdb: SeedSpaceMembership: seeding space_members: %v", err)
	}

	return spaceID
}

// SeedCertificate creates a throwaway learning track (which
// learning_certificates.track_id requires) and issues a certificate on it
// to userID under tenantID, returning the new certificate id.
func SeedCertificate(t *testing.T, d *db.DB, tenantID, userID pgtype.UUID) pgtype.UUID {
	t.Helper()

	ctx := t.Context()

	trackID := mustUUID()
	trackSlug := "test-track-" + uuid.New().String()

	_, err := d.Writer.Exec(ctx,
		`INSERT INTO learning_tracks (id, tenant_id, title, slug) VALUES ($1, $2, 'Test Track', $3)`,
		trackID, tenantID, trackSlug)
	if err != nil {
		t.Fatalf("testdb: SeedCertificate: seeding learning_tracks: %v", err)
	}

	certID := mustUUID()
	certNumber := fmt.Sprintf("TEST-CERT-%s", uuid.New().String())

	_, err = d.Writer.Exec(ctx,
		`INSERT INTO learning_certificates (id, tenant_id, user_id, track_id, certificate_number) VALUES ($1, $2, $3, $4, $5)`,
		certID, tenantID, userID, trackID, certNumber)
	if err != nil {
		t.Fatalf("testdb: SeedCertificate: %v", err)
	}

	return certID
}

// OrgOpts customizes SeedOrganization. Zero-value fields fall back to defaults:
// Slug/Name to a generated unique value, Status to "active", OrgType to
// "business". When OwnerUserID is not Valid, a throwaway active owner user is
// created (organizations.owner_user_id is NOT NULL REFERENCES users(id)).
type OrgOpts struct {
	Slug        string
	Name        string
	Status      string
	OrgType     string
	Verified    bool
	OwnerUserID pgtype.UUID
}

// SeedOrganization inserts an organizations row under tenantID and returns its id.
func SeedOrganization(t *testing.T, d *db.DB, tenantID pgtype.UUID, o OrgOpts) pgtype.UUID {
	t.Helper()

	ctx := t.Context()
	orgID := mustUUID()

	owner := o.OwnerUserID
	if !owner.Valid {
		owner = SeedUser(t, d, tenantID, UserOpts{})
	}
	slug := o.Slug
	if slug == "" {
		slug = "org-" + uuid.New().String()
	}
	name := o.Name
	if name == "" {
		name = slug
	}
	status := o.Status
	if status == "" {
		status = "active"
	}
	orgType := o.OrgType
	if orgType == "" {
		orgType = "business"
	}

	_, err := d.Writer.Exec(ctx,
		`INSERT INTO organizations (id, tenant_id, owner_user_id, name, slug, org_type, status, verified)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		orgID, tenantID, owner, name, slug, orgType, status, o.Verified)
	if err != nil {
		t.Fatalf("testdb: SeedOrganization: %v", err)
	}
	return orgID
}

// SeedOrgMembership inserts an organization_members row. role defaults to "member".
func SeedOrgMembership(t *testing.T, d *db.DB, orgID, userID pgtype.UUID, role string) {
	t.Helper()
	if role == "" {
		role = "member"
	}
	_, err := d.Writer.Exec(t.Context(),
		`INSERT INTO organization_members (organization_id, user_id, role) VALUES ($1, $2, $3)`,
		orgID, userID, role)
	if err != nil {
		t.Fatalf("testdb: SeedOrgMembership: %v", err)
	}
}

// CalendarEntryOpts customizes SeedCalendarEntry. Zero-value fields fall
// back to sensible defaults: Status to "published", Category to "other",
// OccursAt to 30 days from now, SourceURL to a placeholder.
// Ref: japaconnect-engineering/sections/86-community-calendar.md
type CalendarEntryOpts struct {
	Title              string
	Category           string
	Status             string
	SourceURL          string
	IsRecurring        bool
	RecurrencePattern  string
	OccursAt           string // RFC 3339; defaults to now+30d
	DestinationSpaceID pgtype.UUID
}

// SeedCalendarEntry inserts a calendar_entries row under tenantID with
// submittedBy set to userID, returning the new entry id. The destination
// space must already exist (use SeedSpaceMembership first).
func SeedCalendarEntry(t *testing.T, d *db.DB, tenantID, userID pgtype.UUID, o CalendarEntryOpts) pgtype.UUID {
	t.Helper()

	title := o.Title
	if title == "" {
		title = "Test Calendar Entry"
	}
	category := o.Category
	if category == "" {
		category = "other"
	}
	status := o.Status
	if status == "" {
		status = "published"
	}
	sourceURL := o.SourceURL
	if sourceURL == "" {
		sourceURL = "https://example.com/source"
	}

	entryID := mustUUID()

	// When OccursAt is a literal timestamp, use a parameterized query;
	// otherwise use the SQL expression (now() + interval).
	if o.OccursAt != "" {
		_, err := d.Writer.Exec(t.Context(), `
			INSERT INTO calendar_entries (
				id, tenant_id, title, category, status, source_url,
				destination_space_id, submitted_by, occurs_at, is_recurring,
				recurrence_pattern, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), now())
		`, entryID, tenantID, title, category, status, sourceURL,
			o.DestinationSpaceID, userID, o.OccursAt, o.IsRecurring,
			nilIfEmpty(o.RecurrencePattern))
		if err != nil {
			t.Fatalf("testdb: SeedCalendarEntry: %v", err)
		}
	} else {
		var recPattern interface{}
		if o.RecurrencePattern != "" {
			recPattern = o.RecurrencePattern
		}
		_, err := d.Writer.Exec(t.Context(), `
			INSERT INTO calendar_entries (
				id, tenant_id, title, category, status, source_url,
				destination_space_id, submitted_by, occurs_at, is_recurring,
				recurrence_pattern, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now() + interval '30 days', $9, $10, now(), now())
		`, entryID, tenantID, title, category, status, sourceURL,
			o.DestinationSpaceID, userID, o.IsRecurring, recPattern)
		if err != nil {
			t.Fatalf("testdb: SeedCalendarEntry: %v", err)
		}
	}

	return entryID
}

// nilIfEmpty returns a nil interface{} for empty strings, suitable for
// nullable columns.
func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
