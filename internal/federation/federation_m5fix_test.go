package federation

// Ref: japaconnect-engineering/sections/126-activitypub-federation.md (M5)
// Unit coverage for the Pass-36 audit fixes — self-follow guards,
// notification deep links, the remote `updated` clamp, and the
// resolve_actor enqueue seam. DB-backed paths ride the
// JC_TEST_DATABASE_URL suite.

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

func TestIsLocalHost(t *testing.T) {
	s := testService() // api.japa.to / japa.to / handle japa.to
	for _, host := range []string{
		"api.japa.to", "japa.to", "JAPA.TO", "Api.Japa.To",
		"api.japa.to:443", // explicit port still names the host
	} {
		if !s.isLocalHost(host) {
			t.Errorf("isLocalHost(%q) = false, want true", host)
		}
	}
	for _, host := range []string{
		"remote.example", "notjapa.to", "japa.to.evil.com",
		"sub.japa.to", // subdomains are not the apex identity
		"localhost", "",
	} {
		if s.isLocalHost(host) {
			t.Errorf("isLocalHost(%q) = true, want false", host)
		}
	}
}

// TestResolveTargetURLRejectsLocal covers the acct-path self-follow hole:
// the local check must fire before instanceAllowed/WebFinger, and the
// https path must refuse all three local surfaces.
func TestResolveTargetURLRejectsLocal(t *testing.T) {
	s := testService()
	for _, target := range []string{
		"alice@japa.to", "acct:alice@japa.to", "@alice@japa.to",
		"ALICE@JAPA.TO", "alice@api.japa.to",
		"https://japa.to/users/alice",
		"https://api.japa.to/federation/actors/alice",
	} {
		if _, err := s.resolveTargetURL(context.Background(), target); !errors.Is(err, ErrInvalidTarget) {
			t.Errorf("resolveTargetURL(%q) err = %v, want ErrInvalidTarget", target, err)
		}
	}
}

// TestFetchTargetActorRejectsLocal covers the second self-follow gate: a
// WebFinger answer that hands back our own actor URL must never upsert a
// local identity into remote_actors.
func TestFetchTargetActorRejectsLocal(t *testing.T) {
	s := testService()
	for _, actorURL := range []string{
		"https://japa.to/users/alice",
		"https://api.japa.to/federation/actors/alice",
		"http://japa.to/users/alice", // non-https is refused before the local check
	} {
		if _, _, err := s.fetchTargetActor(context.Background(), actorURL); !errors.Is(err, ErrInvalidTarget) {
			t.Errorf("fetchTargetActor(%q) err = %v, want ErrInvalidTarget", actorURL, err)
		}
	}
}

// TestFollowIntentActionRoute pins the member notification deep link —
// /@username has no route (the published-page catch-all 404s it); the
// member profile lives at /u/{username}.
func TestFollowIntentActionRoute(t *testing.T) {
	s := testService()
	var uid pgtype.UUID
	if err := uid.Scan("00000000-0000-0000-0000-000000000001"); err != nil {
		t.Fatal(err)
	}
	acct := "bob@remote.example"
	intent := s.followIntent(
		gen.FederatedActor{UserID: uid},
		gen.GetActorProfileByUsernameRow{Username: "alice"},
		gen.RemoteActor{ActorUrl: "https://remote.example/users/bob", Acct: &acct},
	)
	if intent.action != "/u/alice" {
		t.Errorf("follow intent action = %q, want /u/alice", intent.action)
	}
	if intent.body == "" || intent.title == "" {
		t.Error("follow intent text fields must stay populated")
	}
}

// TestFollowAcceptIntentActionRoute pins the outbound-accept deep link —
// /me/federation is not a route; the member's federation surface is
// /settings/federation.
func TestFollowAcceptIntentActionRoute(t *testing.T) {
	var uid pgtype.UUID
	if err := uid.Scan("00000000-0000-0000-0000-000000000001"); err != nil {
		t.Fatal(err)
	}
	acct := "bob@remote.example"
	intent := followAcceptIntent(uid, &gen.RemoteActor{
		ActorUrl: "https://remote.example/users/bob", Acct: &acct,
	})
	if intent.action != "/settings/federation" {
		t.Errorf("accept intent action = %q, want /settings/federation", intent.action)
	}
}

// TestRemoteEditedAtClamp — the `updated` stamp gets the same far-future
// guard as `published`: remote-claimed edit times clamp to receipt time.
func TestRemoteEditedAtClamp(t *testing.T) {
	future := time.Now().Add(365 * 24 * time.Hour).UTC().Format(time.RFC3339)
	got := remoteEditedAt(map[string]any{"updated": future})
	if !got.Valid || got.Time.After(time.Now().Add(time.Minute)) {
		t.Errorf("future updated not clamped: %v", got.Time)
	}
	past := "2024-01-15T10:00:00Z"
	got = remoteEditedAt(map[string]any{"updated": past})
	if !got.Valid || got.Time.Format(time.RFC3339) != past {
		t.Errorf("past updated altered: %v", got.Time)
	}
	// Missing/malformed stamps fall back to now, never zero/invalid.
	got = remoteEditedAt(map[string]any{})
	if !got.Valid || got.Time.After(time.Now().Add(time.Minute)) {
		t.Errorf("missing updated: %v", got)
	}
}

// TestEnqueueResolveActorNilEnqueuer — services built without the worker
// seam (tests, in-tx dispatch) degrade to cache-TTL refresh, not a panic.
func TestEnqueueResolveActorNilEnqueuer(t *testing.T) {
	s := testService()
	if err := s.enqueueResolveActor(context.Background(), "https://remote.example/users/a"); err != nil {
		t.Errorf("nil enqueuer should no-op, got %v", err)
	}
}

// TestEnqueueResolveActorSendsURL — a wired enqueuer receives the exact
// actor URL the resolve task will re-fetch.
func TestEnqueueResolveActorSendsURL(t *testing.T) {
	s := testService()
	var got string
	s.SetEnqueuer(enqueuerStub{resolve: func(_ context.Context, actorURL string) error {
		got = actorURL
		return nil
	}})
	const want = "https://remote.example/users/a"
	if err := s.enqueueResolveActor(context.Background(), want); err != nil {
		t.Fatalf("enqueueResolveActor: %v", err)
	}
	if got != want {
		t.Errorf("enqueuer saw %q, want %q", got, want)
	}
}

// TestStatusForMovedToURL — the member-facing status surfaces the
// migration redirect so the UI can render the moved banner.
func TestStatusForMovedToURL(t *testing.T) {
	s := testService()
	actor := gen.FederatedActor{
		ActorUrl:          "https://api.japa.to/federation/actors/alice",
		PreferredUsername: "alice",
	}
	st := s.statusFor(context.Background(), actor, "alice")
	if st.MovedToURL != nil {
		t.Errorf("moved_to_url should be nil, got %v", *st.MovedToURL)
	}
	target := "https://remote.example/users/alice"
	actor.MovedToUrl = &target
	st = s.statusFor(context.Background(), actor, "alice")
	if st.MovedToURL == nil || *st.MovedToURL != target {
		t.Errorf("moved_to_url = %v, want %q", st.MovedToURL, target)
	}
}

// enqueuerStub satisfies TaskEnqueuer for the seam tests — only the
// resolve arm is exercised here.
type enqueuerStub struct {
	resolve func(ctx context.Context, actorURL string) error
}

func (enqueuerStub) EnqueueDelivery(context.Context, pgtype.UUID) error { return nil }
func (enqueuerStub) EnqueueDeliveryAfter(context.Context, pgtype.UUID, time.Duration) error {
	return nil
}
func (enqueuerStub) EnqueueInbound(context.Context, pgtype.UUID) error { return nil }
func (e enqueuerStub) EnqueueResolveActor(ctx context.Context, actorURL string) error {
	if e.resolve == nil {
		return fmt.Errorf("resolve not stubbed")
	}
	return e.resolve(ctx, actorURL)
}
