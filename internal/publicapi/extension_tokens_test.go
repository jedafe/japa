package publicapi

import (
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

func TestExtensionTokenName(t *testing.T) {
	// Generated suffix when install_id is absent.
	name, err := extensionTokenName("")
	if err != nil {
		t.Fatalf("empty install id: %v", err)
	}

	if got := name[len(extensionTokenPrefix):]; uuid.Validate(got) != nil {
		t.Fatalf("generated name suffix is not a UUID: %q", name)
	}

	// Provided UUID is used verbatim.
	id := uuid.NewString()

	name, err = extensionTokenName(id)
	if err != nil {
		t.Fatalf("valid install id: %v", err)
	}

	if name != extensionTokenPrefix+id {
		t.Fatalf("unexpected name %q", name)
	}

	// Non-UUID rejected.
	if _, err := extensionTokenName("not-a-uuid"); !errors.Is(err, ErrInvalidInstallID) {
		t.Fatalf("expected ErrInvalidInstallID, got %v", err)
	}
}

func TestExtensionTokenScopes(t *testing.T) {
	for _, want := range []string{"api.read.digest", "api.read.content", "api.read.me", "api.read.safety"} {
		if !slices.Contains(extensionTokenScopes, want) {
			t.Fatalf("scope set missing %q: %v", want, extensionTokenScopes)
		}
	}

	for _, banned := range []string{"agent.use", "api.read.search", "api.read.brief", "api.write.content"} {
		if slices.Contains(extensionTokenScopes, banned) {
			t.Fatalf("scope set must not include %q", banned)
		}
	}
}

func TestIsExtensionToken(t *testing.T) {
	if !isExtensionToken("extension:abc") {
		t.Fatal("prefix name should match")
	}

	for _, name := range []string{"", "extension", "my-extension:abc", "prod key"} {
		if isExtensionToken(name) {
			t.Fatalf("unexpected match for %q", name)
		}
	}
}

// TestExtensionTokenIntegration exercises mint → validate → list → revoke
// against a real Postgres. Skipped unless JC_DATABASE_URL is set.
func TestExtensionTokenIntegration(t *testing.T) {
	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_DATABASE_URL not set; skipping integration test")
	}

	database, err := db.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}

	t.Cleanup(func() { database.Writer.Close(); database.Reader.Close() })

	ctx := t.Context()
	svc := NewService(NewStore(database.Writer))

	var userID pgtype.UUID
	if err := database.Writer.QueryRow(ctx, `INSERT INTO users (id,email,username,profile_type,status,created_at,updated_at,follower_count,force_password_change)
		VALUES (gen_random_uuid(),gen_random_uuid()::text||'@test.local','ext_'||replace(gen_random_uuid()::text,'-',''),'regular','active',now(),now(),0,false) RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("user fixture: %v", err)
	}

	t.Cleanup(func() {
		database.Writer.Exec(ctx, `DELETE FROM api_keys WHERE user_id=$1`, userID)
		database.Writer.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	})

	// Mint with an install ID.
	installID := uuid.NewString()

	resp, err := svc.CreateExtensionToken(ctx, userID, installID)
	if err != nil {
		t.Fatalf("CreateExtensionToken: %v", err)
	}

	if resp.Name != "extension:"+installID {
		t.Fatalf("unexpected name %q", resp.Name)
	}

	if resp.Key == "" || len(resp.Key) < 20 {
		t.Fatalf("expected full key in response, got %q", resp.Key)
	}

	for _, want := range []string{"api.read.digest", "api.read.content", "api.read.me", "api.read.safety"} {
		if !slices.Contains(resp.Scopes, want) {
			t.Fatalf("minted key missing scope %q: %v", want, resp.Scopes)
		}
	}
	// "standard" is requested but capRateLimitTier caps by the owner's
	// unified tier — a regular-tier fixture user gets "basic".
	if resp.RateLimitTier != "basic" {
		t.Fatalf("expected tier capped to basic for regular user, got %q", resp.RateLimitTier)
	}

	var keyID pgtype.UUID
	if err := keyID.Scan(resp.ID); err != nil {
		t.Fatalf("parse key id: %v", err)
	}

	// The minted key validates and carries the extension scope set.
	keyCtx, err := svc.ValidateAPIKey(ctx, resp.Key)
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}

	if keyCtx.UserID != userID {
		t.Fatalf("key owner mismatch: %v", keyCtx.UserID)
	}

	if !slices.Contains(keyCtx.Scopes, "api.read.digest") {
		t.Fatalf("validated key missing digest scope: %v", keyCtx.Scopes)
	}

	// Mint without an install ID — generated suffix.
	resp2, err := svc.CreateExtensionToken(ctx, userID, "")
	if err != nil {
		t.Fatalf("CreateExtensionToken (no install id): %v", err)
	}

	if !isExtensionToken(resp2.Name) || resp2.Name == resp.Name {
		t.Fatalf("unexpected generated name %q", resp2.Name)
	}

	// Invalid install ID.
	if _, err := svc.CreateExtensionToken(ctx, userID, "bogus"); !errors.Is(err, ErrInvalidInstallID) {
		t.Fatalf("expected ErrInvalidInstallID, got %v", err)
	}

	// A non-extension key must not appear in the extension list.
	if _, err := svc.CreateAPIKey(ctx, userID, CreateAPIKeyInput{Name: "dev-portal-key", Scopes: []string{"api.read.*"}}); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	list, err := svc.ListExtensionTokens(ctx, userID)
	if err != nil {
		t.Fatalf("ListExtensionTokens: %v", err)
	}

	if len(list) != 2 {
		t.Fatalf("expected 2 extension tokens, got %d: %+v", len(list), list)
	}

	for _, k := range list {
		if !isExtensionToken(k.Name) {
			t.Fatalf("non-extension key leaked into list: %q", k.Name)
		}
	}

	// Revocation through the ordinary api-keys path.
	if err := svc.RevokeAPIKey(ctx, userID, keyID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}

	list, err = svc.ListExtensionTokens(ctx, userID)
	if err != nil {
		t.Fatalf("ListExtensionTokens after revoke: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 token after revoke, got %d", len(list))
	}
}
