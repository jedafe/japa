package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/events"
)

// fakeDiscoveryDeps records follower-list calls and returns a canned cohort.
type fakeDiscoveryDeps struct {
	followers  []pgtype.UUID
	content    gen.GetContentByIDRow
	contentErr error
	listLimit  int32
}

func (f *fakeDiscoveryDeps) ListSpaceFollowerIDs(_ context.Context, _ string, _ pgtype.UUID, limit int32) ([]pgtype.UUID, error) {
	f.listLimit = limit
	return f.followers, nil
}

func (f *fakeDiscoveryDeps) GetContentByID(_ context.Context, _ pgtype.UUID) (gen.GetContentByIDRow, error) {
	return f.content, f.contentErr
}

type createdNotif struct {
	recipientID pgtype.UUID
	nType       string
	subjectType string
	subjectID   pgtype.UUID
	title       string
	body        string
	actionURL   string
}

type fakeDiscoveryNotifier struct {
	unread   map[string]bool
	created  []createdNotif
	hasCalls int
}

func (f *fakeDiscoveryNotifier) HasUnreadNotification(_ context.Context, recipientID pgtype.UUID, nType, subjectType string, subjectID pgtype.UUID) (bool, error) {
	f.hasCalls++
	return f.unread[recipientID.String()], nil
}

func (f *fakeDiscoveryNotifier) CreateNotification(_ context.Context, recipientID pgtype.UUID, nType, actorType string, _ pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error {
	f.created = append(f.created, createdNotif{
		recipientID: recipientID,
		nType:       nType,
		subjectType: subjectType,
		subjectID:   subjectID,
		title:       title,
		body:        body,
		actionURL:   actionURL,
	})
	return nil
}

func discoveryTask(t *testing.T, evt events.DomainEvent) *asynq.Task {
	t.Helper()
	payload, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return asynq.NewTask(TypeIntelDiscoveryFanout, payload)
}

func TestIntelDiscoveryFanout_NotifiesFollowers(t *testing.T) {
	follower1 := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	follower2 := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	spaceID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	contentID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	domainID := uuid.New()

	deps := &fakeDiscoveryDeps{
		followers: []pgtype.UUID{follower1, follower2},
		content: gen.GetContentByIDRow{
			ID:   contentID,
			Slug: "canada-grants-guide",
		},
	}
	notifier := &fakeDiscoveryNotifier{unread: map[string]bool{}}

	RegisterIntelDiscoveryFanout(deps, notifier)
	t.Cleanup(func() { RegisterIntelDiscoveryFanout(nil, nil) })

	payload, _ := json.Marshal(map[string]any{
		"domain_id":        domainID.String(),
		"country_space_id": uuid.UUID(spaceID.Bytes).String(),
		"country":          "Canada",
		"content_id":       uuid.UUID(contentID.Bytes).String(),
	})
	task := discoveryTask(t, events.DomainEvent{
		Type:       "grants.discovered",
		EntityType: "grants",
		EntityID:   domainID,
		Payload:    payload,
		OccurredAt: time.Now().UTC(),
	})

	if err := HandleIntelDiscoveryFanout(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if len(notifier.created) != 2 {
		t.Fatalf("notifications created = %d, want 2", len(notifier.created))
	}

	for _, n := range notifier.created {
		if n.nType != "intel.discovery" {
			t.Errorf("nType = %q, want intel.discovery", n.nType)
		}
		if n.subjectType != "grants" {
			t.Errorf("subjectType = %q, want grants", n.subjectType)
		}
		if n.actionURL != "/content/canada-grants-guide" {
			t.Errorf("actionURL = %q, want /content/canada-grants-guide", n.actionURL)
		}
		if n.title != "New grants in Canada" {
			t.Errorf("title = %q, want 'New grants in Canada'", n.title)
		}
	}
}

func TestIntelDiscoveryFanout_SkipsUnreadDupes(t *testing.T) {
	follower1 := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	follower2 := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	spaceID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	domainID := uuid.New()

	deps := &fakeDiscoveryDeps{followers: []pgtype.UUID{follower1, follower2}}
	notifier := &fakeDiscoveryNotifier{unread: map[string]bool{follower1.String(): true}}

	RegisterIntelDiscoveryFanout(deps, notifier)
	t.Cleanup(func() { RegisterIntelDiscoveryFanout(nil, nil) })

	payload, _ := json.Marshal(map[string]any{
		"domain_id":        domainID.String(),
		"country_space_id": uuid.UUID(spaceID.Bytes).String(),
	})
	task := discoveryTask(t, events.DomainEvent{
		Type:       "scholarship.discovered",
		EntityType: "scholarship",
		EntityID:   domainID,
		Payload:    payload,
		OccurredAt: time.Now().UTC(),
	})

	if err := HandleIntelDiscoveryFanout(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if len(notifier.created) != 1 {
		t.Fatalf("notifications created = %d, want 1 (follower1 deduped)", len(notifier.created))
	}
	if notifier.created[0].recipientID != follower2 {
		t.Errorf("notified wrong follower")
	}
	if notifier.created[0].actionURL != "/sources" {
		t.Errorf("actionURL = %q, want /sources fallback", notifier.created[0].actionURL)
	}
}

func TestIntelDiscoveryFanout_NoSpaceSkips(t *testing.T) {
	deps := &fakeDiscoveryDeps{followers: []pgtype.UUID{{Bytes: uuid.New(), Valid: true}}}
	notifier := &fakeDiscoveryNotifier{unread: map[string]bool{}}

	RegisterIntelDiscoveryFanout(deps, notifier)
	t.Cleanup(func() { RegisterIntelDiscoveryFanout(nil, nil) })

	payload, _ := json.Marshal(map[string]any{
		"domain_id": uuid.New().String(),
	})
	task := discoveryTask(t, events.DomainEvent{
		Type:       "news.discovered",
		EntityType: "news",
		EntityID:   uuid.New(),
		Payload:    payload,
		OccurredAt: time.Now().UTC(),
	})

	if err := HandleIntelDiscoveryFanout(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if deps.listLimit != 0 {
		t.Errorf("follower lookup ran without a space id")
	}
	if len(notifier.created) != 0 {
		t.Errorf("notifications created without a space cohort")
	}
}

func TestIntelDiscoveryFanout_FallsBackToContentGeoSpace(t *testing.T) {
	follower := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	spaceID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	contentID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	deps := &fakeDiscoveryDeps{
		followers: []pgtype.UUID{follower},
		content: gen.GetContentByIDRow{
			ID:         contentID,
			Slug:       "ghana-banking",
			GeoSpaceID: spaceID,
		},
	}
	notifier := &fakeDiscoveryNotifier{unread: map[string]bool{}}

	RegisterIntelDiscoveryFanout(deps, notifier)
	t.Cleanup(func() { RegisterIntelDiscoveryFanout(nil, nil) })

	// Content-projection domains emit .discovered with no country_space_id —
	// the fan-out must resolve it from the contents row's geo_space_id.
	payload, _ := json.Marshal(map[string]any{
		"domain_id":  uuid.New().String(),
		"content_id": uuid.UUID(contentID.Bytes).String(),
	})
	task := discoveryTask(t, events.DomainEvent{
		Type:       "banking.discovered",
		EntityType: "banking",
		EntityID:   uuid.New(),
		Payload:    payload,
		OccurredAt: time.Now().UTC(),
	})

	if err := HandleIntelDiscoveryFanout(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(notifier.created) != 1 {
		t.Fatalf("notifications created = %d, want 1 via content geo_space fallback", len(notifier.created))
	}
	if notifier.created[0].actionURL != "/content/ghana-banking" {
		t.Errorf("actionURL = %q, want /content/ghana-banking", notifier.created[0].actionURL)
	}
}
