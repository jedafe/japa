package federation

// §126 M3 DB integration tests — instance actor, space Group actors,
// relay subscription lifecycle, authorized-fetch gate, hashtag emission.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// settingsStub implements SettingsReader with an in-memory map.
type settingsStub struct{ kv map[string]string }

func (s *settingsStub) GetSetting(ctx context.Context, key string) (string, error) {
	v, ok := s.kv[key]
	if !ok {
		return "", fmt.Errorf("missing")
	}
	return v, nil
}
func (s *settingsStub) GetSettingBool(ctx context.Context, key string) (bool, error) {
	return s.kv[key] == "true", nil
}
func (s *settingsStub) GetSettingInt(ctx context.Context, key string) (int, error) {
	var n int
	fmt.Sscanf(s.kv[key], "%d", &n)
	return n, nil
}

// newSpace seeds a minimal member-created space (level='custom' + a
// creator — the §05 member_created predicate Group actors require).
func newSpace(t *testing.T, ctx context.Context, d *db.DB, visibility string) (pgtype.UUID, string) {
	t.Helper()
	creatorID, _ := newFederationUser(t, ctx, d, "active")
	return newSpaceLevel(t, ctx, d, creatorID, "custom", visibility)
}

// newSpaceLevel seeds a space at an arbitrary level — platform/geo
// spaces (non-'custom' levels, no creator) must fail Group provisioning.
func newSpaceLevel(t *testing.T, ctx context.Context, d *db.DB, creatorID pgtype.UUID, level, visibility string) (pgtype.UUID, string) {
	t.Helper()
	slug := fmt.Sprintf("fedspace-%d", time.Now().UnixNano())
	var id pgtype.UUID
	err := d.Writer.QueryRow(ctx, `
		INSERT INTO spaces (space_type_id, name, slug, level, visibility, status, creator_user_id)
		VALUES ('00000000-0000-0000-0000-000000000060', 'Fed Space', $1, $2, $3, 'active', $4)
		RETURNING id`, slug, level, visibility, creatorID).Scan(&id)
	if err != nil {
		t.Fatalf("insert space: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM spaces WHERE id = $1`, id)
	})
	return id, slug
}

// newRelayRow seeds a federation_relays row directly (SubscribeRelay's
// remote fetch path is exercised by the fetch-doc layer, not here).
func newRelayRow(t *testing.T, ctx context.Context, svc *Service, actorURL, status string) gen.FederationRelay {
	t.Helper()
	followID := svc.apiBase() + "/federation/instance#relay-follow-" + fmt.Sprint(time.Now().UnixNano())
	host := strings.TrimPrefix(strings.SplitN(actorURL, "/", 4)[2], "")
	row, err := svc.store.upsertRelay(ctx, gen.UpsertFederationRelayParams{
		ActorUrl:   actorURL,
		InboxUrl:   "https://" + host + "/inbox",
		Domain:     host,
		FollowApID: &followID,
	})
	if err != nil {
		t.Fatalf("seed relay: %v", err)
	}
	if status != "pending" {
		if err := svc.store.write.SetFederationRelayStatus(ctx, gen.SetFederationRelayStatusParams{
			ID: row.ID, Status: status,
		}); err != nil {
			t.Fatalf("relay status: %v", err)
		}
		row.Status = status
	}
	t.Cleanup(func() {
		_, _ = svc.db.Writer.Exec(context.Background(), `DELETE FROM federation_relays WHERE id = $1`, row.ID)
	})
	return row
}

func TestDBInstanceActorLifecycle(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)
	defer func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM federated_actors WHERE kind = 'application'`)
	}()

	a1, err := svc.ensureInstanceActor(ctx)
	if err != nil {
		t.Fatalf("ensureInstanceActor: %v", err)
	}
	if a1.Kind != ActorKindApplication || a1.UserID.Valid {
		t.Fatalf("bad instance actor: kind=%s user=%v", a1.Kind, a1.UserID)
	}
	if a1.ActorUrl != "https://api.test.dev/federation/instance" {
		t.Errorf("actor url: %s", a1.ActorUrl)
	}
	// Singleton — second ensure returns the same row.
	a2, err := svc.ensureInstanceActor(ctx)
	if err != nil || a2.ID != a1.ID {
		t.Fatalf("singleton violation: %v vs %v", a1.ID, a2.ID)
	}

	doc, err := svc.InstanceDocument(ctx)
	if err != nil {
		t.Fatalf("InstanceDocument: %v", err)
	}
	if doc["type"] != "Application" || doc["id"] != a1.ActorUrl {
		t.Errorf("doc: %+v", doc)
	}
	pk, _ := doc["publicKey"].(map[string]any)
	if pk["owner"] != a1.ActorUrl {
		t.Errorf("publicKey owner: %v", pk)
	}

	// WebFinger acct:test.dev@test.dev → instance JRD.
	jrd, err := svc.Webfinger(ctx, "acct:test.dev@test.dev")
	if err != nil {
		t.Fatalf("instance webfinger: %v", err)
	}
	if jrd["subject"] != "acct:test.dev@test.dev" {
		t.Errorf("subject: %v", jrd["subject"])
	}

	// Follow{instance} auto-accepts and ledgers the edge + Accept delivery.
	remote, remoteURL := newRemoteActor(t, ctx, svc, "inst-follow.example")
	followAct := map[string]any{
		"id":     fmt.Sprintf("https://inst-follow.example/activities/%d", time.Now().UnixNano()),
		"type":   "Follow",
		"actor":  remoteURL,
		"object": a1.ActorUrl,
	}
	row := runInbound(t, ctx, svc, followAct)
	if row.Status != "processed" {
		t.Fatalf("instance follow status=%s err=%v", row.Status, row.Error)
	}
	var followStatus string
	if err := d.Reader.QueryRow(ctx,
		`SELECT status FROM federation_follows WHERE local_actor_id = $1 AND remote_actor_id = $2`,
		a1.ID, remote.ID).Scan(&followStatus); err != nil {
		t.Fatalf("follow edge: %v", err)
	}
	if followStatus != "accepted" {
		t.Errorf("edge status: %s", followStatus)
	}
	// Accept delivery signed by the instance actor.
	var n int64
	if err := d.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM federation_deliveries WHERE local_actor_id = $1 AND activity_type = 'Accept'`,
		a1.ID).Scan(&n); err != nil {
		t.Fatalf("deliveries: %v", err)
	}
	if n == 0 {
		t.Error("no Accept delivery ledgered for instance follow")
	}
	// No follows-mirror for non-person actors.
	if err := d.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM follows WHERE follower_remote_actor_id = $1`, remote.ID).Scan(&n); err == nil && n != 0 {
		t.Errorf("follows mirror written for instance actor")
	}
}

func TestDBGroupActorLifecycle(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)
	spaceID, slug := newSpace(t, ctx, d, "public")
	adminID, _ := newFederationUser(t, ctx, d, "active")

	actor, err := svc.ProvisionGroupActor(ctx, spaceID, adminID)
	if err != nil {
		t.Fatalf("ProvisionGroupActor: %v", err)
	}
	if actor.Kind != ActorKindGroup || actor.ActorUrl != "https://api.test.dev/federation/groups/"+slug {
		t.Fatalf("bad group actor: %+v", actor)
	}
	// Idempotent re-provision.
	again, err := svc.ProvisionGroupActor(ctx, spaceID, adminID)
	if err != nil || again.ID != actor.ID {
		t.Fatalf("re-provision: %v %v", again, err)
	}

	doc, err := svc.GroupDocument(ctx, slug)
	if err != nil {
		t.Fatalf("GroupDocument: %v", err)
	}
	if doc["type"] != "Group" || doc["preferredUsername"] != slug {
		t.Errorf("group doc: %+v", doc)
	}
	if doc["url"] != "https://test.dev/spaces/"+slug {
		t.Errorf("group url: %v", doc["url"])
	}

	// WebFinger acct:!slug@test.dev → group JRD.
	jrd, err := svc.Webfinger(ctx, "acct:!"+slug+"@test.dev")
	if err != nil {
		t.Fatalf("group webfinger: %v", err)
	}
	if jrd["subject"] != "acct:!"+slug+"@test.dev" {
		t.Errorf("subject: %v", jrd["subject"])
	}

	// Follow{group} auto-accepts (no profile approval for spaces).
	remote, remoteURL := newRemoteActor(t, ctx, svc, "grp-follow.example")
	row := runInbound(t, ctx, svc, map[string]any{
		"id": fmt.Sprintf("https://grp-follow.example/activities/%d", time.Now().UnixNano()), "type": "Follow",
		"actor": remoteURL, "object": actor.ActorUrl,
	})
	if row.Status != "processed" {
		t.Fatalf("group follow status=%s", row.Status)
	}
	var fs string
	if err := d.Reader.QueryRow(ctx,
		`SELECT status FROM federation_follows WHERE local_actor_id = $1 AND remote_actor_id = $2`,
		actor.ID, remote.ID).Scan(&fs); err != nil || fs != "accepted" {
		t.Fatalf("group edge: %v %s", err, fs)
	}

	// Tombstone → doc 410 path + Delete fanout rows.
	if err := svc.TombstoneGroupActor(ctx, actor.ID); err != nil {
		t.Fatalf("TombstoneGroupActor: %v", err)
	}
	if _, err := svc.GroupDocument(ctx, slug); err != ErrGone {
		t.Fatalf("tombstoned group doc err: %v", err)
	}
	if tomb := svc.GroupTombstone(ctx, slug); tomb == nil || tomb["type"] != "Tombstone" {
		t.Error("no tombstone body")
	}
	var n int64
	if err := d.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM federation_deliveries WHERE local_actor_id = $1 AND activity_type = 'Delete'`,
		actor.ID).Scan(&n); err != nil || n == 0 {
		t.Error("no Delete{Group} delivery ledgered")
	}
}

func TestDBGroupSpaceEligibilityGates(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)
	adminID, _ := newFederationUser(t, ctx, d, "active")

	// Private member-created space — visibility gate.
	privID, _ := newSpace(t, ctx, d, "private")
	if _, err := svc.ProvisionGroupActor(ctx, privID, adminID); err != ErrConflict {
		t.Fatalf("private space provision err: %v", err)
	}

	// Platform/geo space (level='country', no creator) — member-created gate.
	var geoID pgtype.UUID
	if err := d.Writer.QueryRow(ctx, `
		INSERT INTO spaces (space_type_id, name, slug, level, visibility, status)
		VALUES ('00000000-0000-0000-0000-000000000060', 'Geo Space', $1, 'country', 'public', 'active')
		RETURNING id`, fmt.Sprintf("geospace-%d", time.Now().UnixNano())).Scan(&geoID); err != nil {
		t.Fatalf("insert geo space: %v", err)
	}
	defer func() { _, _ = d.Writer.Exec(context.Background(), `DELETE FROM spaces WHERE id = $1`, geoID) }()
	if _, err := svc.ProvisionGroupActor(ctx, geoID, adminID); err != ErrConflict {
		t.Fatalf("platform space provision err: %v", err)
	}

	// Custom-level space with NULL creator (creator erased) — also ineligible.
	creatorlessID, _ := newSpaceLevel(t, ctx, d, pgtype.UUID{}, "custom", "public")
	if _, err := svc.ProvisionGroupActor(ctx, creatorlessID, adminID); err != ErrConflict {
		t.Fatalf("creatorless space provision err: %v", err)
	}

	// And a provisioned group actor stops resolving when its space leaves
	// the envelope — erase the creator's provenance.
	spaceID, slug := newSpace(t, ctx, d, "public")
	actor, err := svc.ProvisionGroupActor(ctx, spaceID, adminID)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if _, err := svc.GroupDocument(ctx, slug); err != nil {
		t.Fatalf("doc before lapse: %v", err)
	}
	if _, err := d.Writer.Exec(ctx,
		`UPDATE spaces SET creator_user_id = NULL WHERE id = $1`, spaceID); err != nil {
		t.Fatalf("clear creator: %v", err)
	}
	if _, err := svc.GroupDocument(ctx, slug); err != ErrNotFound {
		t.Fatalf("doc after creator lapse: %v (actor %s)", err, actor.ID)
	}
}

func TestDBRelayDispatchAndCourier(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)
	defer func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM federated_actors WHERE kind = 'application'`)
	}()

	relay, _ := newRemoteActor(t, ctx, svc, "relay.example")
	relayRow := newRelayRow(t, ctx, svc, relay.ActorUrl, "pending")
	if svc.isAcceptedRelay(ctx, relay.ActorUrl) {
		t.Fatal("pending relay must not be accepted")
	}

	// Accept{Follow{follow_ap_id}} from the relay flips pending→accepted.
	row := runInbound(t, ctx, svc, map[string]any{
		"id": fmt.Sprintf("https://relay.example/activities/%d", time.Now().UnixNano()), "type": "Accept",
		"actor": relay.ActorUrl,
		"object": map[string]any{
			"id": *relayRow.FollowApID, "type": "Follow",
			"actor": svc.apiBase() + "/federation/instance", "object": asPublic,
		},
	})
	if row.Status != "processed" {
		t.Fatalf("relay accept status=%s err=%v", row.Status, row.Error)
	}
	var st string
	if err := d.Reader.QueryRow(ctx, `SELECT status FROM federation_relays WHERE id = $1`, relayRow.ID).Scan(&st); err != nil || st != "accepted" {
		t.Fatalf("relay status: %v %s", err, st)
	}
	if !svc.isAcceptedRelay(ctx, relay.ActorUrl) {
		t.Fatal("accepted relay not recognized")
	}

	// A third party embedding our follow id cannot flip state.
	_, evilURL := newRemoteActor(t, ctx, svc, "evil-relay.example")
	row = runInbound(t, ctx, svc, map[string]any{
		"id": fmt.Sprintf("https://evil-relay.example/activities/%d", time.Now().UnixNano()), "type": "Reject",
		"actor": evilURL,
		"object": map[string]any{
			"id": *relayRow.FollowApID, "type": "Follow",
			"actor": svc.apiBase() + "/federation/instance", "object": asPublic,
		},
	})
	if row.Status != "rejected" {
		t.Fatalf("third-party reject status=%s", row.Status)
	}
	if err := d.Reader.QueryRow(ctx, `SELECT status FROM federation_relays WHERE id = $1`, relayRow.ID).Scan(&st); err != nil || st != "accepted" {
		t.Fatalf("relay status after spoof: %v %s", err, st)
	}

	// Courier: a relay-signed activity authored by a DIFFERENT remote
	// actor dispatches as the real author — only because the relay is
	// accepted. Like on a local object proves the effective-signer path.
	author, authorURL := newRemoteActor(t, ctx, svc, "author.example")
	userID, _ := newFederationUser(t, ctx, d, "active")
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	contentID, objURL := newFederatedContent(t, ctx, svc, userID)
	likeID := fmt.Sprintf("https://author.example/activities/%d", time.Now().UnixNano())
	payload, _ := json.Marshal(map[string]any{
		"id": likeID, "type": "Like",
		"actor": authorURL, "object": objURL,
	})
	// row.ActorUrl = the SIGNER (relay) — postInbox records the keyId owner.
	inRow, err := svc.store.insertInbound(ctx, gen.InsertFederationInboundParams{
		ActivityApID:   strPtr(likeID),
		ActivityType:   "Like",
		ActorUrl:       strPtr(relay.ActorUrl),
		SignatureValid: true,
		Payload:        payload,
	})
	if err != nil {
		t.Fatalf("courier inbound insert: %v", err)
	}
	if err := svc.ProcessInbound(ctx, inRow.ID); err != nil {
		t.Fatalf("courier ProcessInbound: %v", err)
	}
	var status, reason string
	if err := d.Reader.QueryRow(ctx, `SELECT status, COALESCE(error,'') FROM federation_inbound WHERE id = $1`, inRow.ID).Scan(&status, &reason); err != nil {
		t.Fatalf("inbound read: %v", err)
	}
	if status != "processed" {
		t.Fatalf("courier like status=%s err=%s", status, reason)
	}
	// The reaction is attributed to the real author, not the relay.
	var n int64
	if err := d.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM reactions WHERE entity_type='content' AND entity_id=$1 AND remote_actor_id=$2`,
		contentID, author.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("courier attribution: n=%d err=%v", n, err)
	}

	// Same shape from a NON-relay signer is rejected at the door.
	fakeRelay, _ := newRemoteActor(t, ctx, svc, "not-a-relay.example")
	likeID2 := fmt.Sprintf("https://author.example/activities/%d-x", time.Now().UnixNano())
	inRow2, err := svc.store.insertInbound(ctx, gen.InsertFederationInboundParams{
		ActivityApID:   strPtr(likeID2),
		ActivityType:   "Like",
		ActorUrl:       strPtr(fakeRelay.ActorUrl),
		SignatureValid: true,
		Payload: func() []byte {
			p, _ := json.Marshal(map[string]any{
				"id": likeID2, "type": "Like",
				"actor": authorURL, "object": objURL,
			})
			return p
		}(),
	})
	if err != nil {
		t.Fatalf("non-relay inbound insert: %v", err)
	}
	if err := svc.ProcessInbound(ctx, inRow2.ID); err != nil {
		t.Fatalf("non-relay ProcessInbound: %v", err)
	}
	if err := d.Reader.QueryRow(ctx, `SELECT status FROM federation_inbound WHERE id = $1`, inRow2.ID).Scan(&status); err != nil || status != "rejected" {
		t.Fatalf("non-relay courier status=%s", status)
	}
}

func TestDBGroupAnnounceFanout(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)
	defer func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM federation_deliveries WHERE activity_type = 'Announce'`)
	}()

	spaceID, _ := newSpace(t, ctx, d, "public")
	adminID, _ := newFederationUser(t, ctx, d, "active")
	group, err := svc.ProvisionGroupActor(ctx, spaceID, adminID)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	// A remote follower of the group.
	remote, _ := newRemoteActor(t, ctx, svc, "grp-fanout.example")
	if _, err := svc.store.write.UpsertFederationFollow(ctx, gen.UpsertFederationFollowParams{
		LocalActorID: group.ID, RemoteActorID: remote.ID, Status: "accepted",
	}); err != nil {
		t.Fatalf("group edge: %v", err)
	}

	// Space-scoped federated content.
	author, _ := newFederationUser(t, ctx, d, "active")
	if _, err := svc.EnableFederation(ctx, author); err != nil {
		t.Fatalf("enable author: %v", err)
	}
	var contentID pgtype.UUID
	if err := d.Writer.QueryRow(ctx, `
		INSERT INTO contents (author_id, space_id, content_type, title, slug, body, body_html,
		                      content_hash, status, visibility, published_at)
		VALUES ($1, $2, 'story', 'Space post', $3, 'hi', '<p>hi</p>', $4, 'published', 'public', now())
		RETURNING id`, author, spaceID,
		fmt.Sprintf("grp-%d", time.Now().UnixNano()),
		fmt.Sprintf("hash-grp-%d", time.Now().UnixNano())).Scan(&contentID); err != nil {
		t.Fatalf("insert content: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM contents WHERE id = $1`, contentID)
	})

	if err := svc.Fanout(ctx, contentID); err != nil {
		t.Fatalf("Fanout: %v", err)
	}
	var announceJSON []byte
	if err := d.Reader.QueryRow(ctx,
		`SELECT activity_json FROM federation_deliveries WHERE local_actor_id = $1 AND activity_type = 'Announce'`,
		group.ID).Scan(&announceJSON); err != nil {
		t.Fatalf("no group Announce delivery: %v", err)
	}
	var act map[string]any
	json.Unmarshal(announceJSON, &act)
	if act["actor"] != group.ActorUrl {
		t.Errorf("announce actor: %v", act["actor"])
	}
}

func TestDBRelayInboxExpansion(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)

	relayActor, _ := newRemoteActor(t, ctx, svc, "relay-exp.example")
	accepted := newRelayRow(t, ctx, svc, relayActor.ActorUrl, "accepted")
	dead, _ := newRemoteActor(t, ctx, svc, "relay-dead.example")
	_ = newRelayRow(t, ctx, svc, dead.ActorUrl, "failed")

	inboxes := svc.relayInboxes(ctx)
	found, foundDead := false, false
	for _, in := range inboxes {
		if in.InboxUrl != nil && *in.InboxUrl == accepted.InboxUrl {
			found = true
		}
		if in.InboxUrl != nil && strings.Contains(*in.InboxUrl, "relay-dead.example") {
			foundDead = true
		}
	}
	if !found {
		t.Error("accepted relay inbox missing from expansion")
	}
	if foundDead {
		t.Error("failed relay present in expansion")
	}
}

// TestDBRelayUnreachableMarksFailed — a delivery dead-lettered for a
// remote-caused reason unwinds the relay behind its inbox so fanout
// stops targeting it (re-subscribe re-arms). Accepted rows unwind on
// any dead delivery; a pending row unwinds only when the dead delivery
// is its own Follow — unrelated sharedInbox traffic must not kill a
// handshake in flight.
func TestDBRelayUnreachableMarksFailed(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)

	userID, _ := newFederationUser(t, ctx, d, "active")
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	signer, err := svc.store.actorByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("signer actor: %v", err)
	}

	// An http inbox makes deliverAP's https-only guard fail the attempt
	// at the transport layer — no remote needed to burn the budget.
	inbox := "http://dead-relay.example/inbox"
	seedRelay := func(actorURL, followID, status string) gen.FederationRelay {
		relay, err := svc.store.upsertRelay(ctx, gen.UpsertFederationRelayParams{
			ActorUrl: actorURL, InboxUrl: inbox, Domain: "dead-relay.example",
			FollowApID: &followID,
		})
		if err != nil {
			t.Fatalf("seed relay: %v", err)
		}
		if err := svc.store.write.SetFederationRelayStatus(ctx, gen.SetFederationRelayStatusParams{
			ID: relay.ID, Status: status,
		}); err != nil {
			t.Fatalf("relay status: %v", err)
		}
		t.Cleanup(func() {
			_, _ = d.Writer.Exec(context.Background(), `DELETE FROM federation_relays WHERE id = $1`, relay.ID)
		})
		return relay
	}
	seedDeadDelivery := func(apID string) pgtype.UUID {
		row, err := svc.store.write.InsertFederationDelivery(ctx, gen.InsertFederationDeliveryParams{
			ActivityApID: apID, ActivityType: "Create",
			ActivityJson: []byte(`{"type":"Create"}`),
			LocalActorID: signer.ID, InboxUrl: inbox,
		})
		if err != nil {
			t.Fatalf("seed delivery: %v", err)
		}
		t.Cleanup(func() {
			_, _ = d.Writer.Exec(context.Background(), `DELETE FROM federation_deliveries WHERE id = $1`, row.ID)
		})
		// One attempt shy of the retry ceiling — the next failure
		// dead-letters the delivery AND runs the unwind check.
		if _, err := d.Writer.Exec(ctx,
			`UPDATE federation_deliveries SET attempts = 7 WHERE id = $1`, row.ID); err != nil {
			t.Fatalf("attempts: %v", err)
		}
		return row.ID
	}
	relayStatus := func(id pgtype.UUID) string {
		var st string
		if err := d.Reader.QueryRow(ctx,
			`SELECT status FROM federation_relays WHERE id = $1`, id).Scan(&st); err != nil {
			t.Fatalf("relay status: %v", err)
		}
		return st
	}
	ts := time.Now().UnixNano()

	// Accepted relay + dead fanout delivery → failed.
	accepted := seedRelay(fmt.Sprintf("https://dead-relay.example/actor-%d", ts),
		fmt.Sprintf("%s/federation/instance#relay-follow-%d", svc.apiBase(), ts), "accepted")
	deliveryID := seedDeadDelivery(fmt.Sprintf("https://api.test.dev/federation/objects/contents/dead-%d", ts))
	if err := svc.Deliver(ctx, deliveryID); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	var dst string
	if err := d.Reader.QueryRow(ctx,
		`SELECT status FROM federation_deliveries WHERE id = $1`, deliveryID).Scan(&dst); err != nil {
		t.Fatalf("delivery status: %v", err)
	}
	if dst != "dead" {
		t.Fatalf("delivery status: %q", dst)
	}
	if st := relayStatus(accepted.ID); st != "failed" {
		t.Fatalf("accepted relay not unwound: %q", st)
	}

	// Pending relay + dead UNRELATED delivery → untouched (the Follow is
	// still in flight — a sharedInbox refusal can't kill the handshake).
	pending := seedRelay(fmt.Sprintf("https://dead-relay.example/actor-p%d", ts),
		fmt.Sprintf("%s/federation/instance#relay-follow-p%d", svc.apiBase(), ts), "pending")
	otherID := seedDeadDelivery(fmt.Sprintf("https://api.test.dev/federation/objects/contents/dead-p%d", ts))
	if err := svc.Deliver(ctx, otherID); err != nil {
		t.Fatalf("Deliver other: %v", err)
	}
	if st := relayStatus(pending.ID); st != "pending" {
		t.Fatalf("pending relay unwound by unrelated delivery: %q", st)
	}

	// Pending relay + dead delivery of its OWN Follow → failed.
	followID := fmt.Sprintf("%s/federation/instance#relay-follow-p%d", svc.apiBase(), ts)
	followDelivery, err := svc.store.write.InsertFederationDelivery(ctx, gen.InsertFederationDeliveryParams{
		ActivityApID: followID, ActivityType: "Follow",
		ActivityJson: []byte(`{"type":"Follow"}`),
		LocalActorID: signer.ID, InboxUrl: inbox,
	})
	if err != nil {
		t.Fatalf("seed follow delivery: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM federation_deliveries WHERE id = $1`, followDelivery.ID)
	})
	if _, err := d.Writer.Exec(ctx,
		`UPDATE federation_deliveries SET attempts = 7 WHERE id = $1`, followDelivery.ID); err != nil {
		t.Fatalf("attempts: %v", err)
	}
	if err := svc.Deliver(ctx, followDelivery.ID); err != nil {
		t.Fatalf("Deliver follow: %v", err)
	}
	if st := relayStatus(pending.ID); st != "failed" {
		t.Fatalf("pending relay not unwound by own dead Follow: %q", st)
	}
}

// TestDBRelayUnwindEndpointGate — the mark query's cause gate:
// accepted relays unwind only on endpoint-level dead letters (a plain
// 4xx is the remote refusing one activity, not going away); pending
// rows unwind whenever their own Follow dies but never on unrelated
// inbox traffic.
func TestDBRelayUnwindEndpointGate(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)
	ts := time.Now().UnixNano()

	inbox := fmt.Sprintf("https://gate-relay-%d.example/inbox", ts)
	followID := fmt.Sprintf("%s/federation/instance#relay-follow-g%d", svc.apiBase(), ts)
	relay, err := svc.store.upsertRelay(ctx, gen.UpsertFederationRelayParams{
		ActorUrl:   fmt.Sprintf("https://gate-relay-%d.example/actor", ts),
		InboxUrl:   inbox,
		Domain:     fmt.Sprintf("gate-relay-%d.example", ts),
		FollowApID: &followID,
	})
	if err != nil {
		t.Fatalf("seed relay: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM federation_relays WHERE id = $1`, relay.ID)
	})
	status := func() string {
		var st string
		if err := d.Reader.QueryRow(ctx,
			`SELECT status FROM federation_relays WHERE id = $1`, relay.ID).Scan(&st); err != nil {
			t.Fatalf("relay status: %v", err)
		}
		return st
	}
	setStatus := func(s string) {
		if err := svc.store.write.SetFederationRelayStatus(ctx, gen.SetFederationRelayStatusParams{
			ID: relay.ID, Status: s,
		}); err != nil {
			t.Fatalf("relay status: %v", err)
		}
	}
	mark := func(apID, cause string, level bool) {
		svc.markRelayUnreachable(ctx, gen.FederationDelivery{
			InboxUrl: inbox, ActivityApID: apID,
		}, cause, level)
	}

	// accepted + activity-level refusal (plain 4xx) → survives.
	setStatus("accepted")
	mark(fmt.Sprintf("https://api.test.dev/objects/%d", ts), "remote 422", false)
	if st := status(); st != "accepted" {
		t.Fatalf("accepted relay unwound by activity-level refusal: %q", st)
	}
	// accepted + endpoint-level dead letter → failed.
	mark(fmt.Sprintf("https://api.test.dev/objects/%d", ts), "remote 410", true)
	if st := status(); st != "failed" {
		t.Fatalf("accepted relay not unwound by endpoint-level dead letter: %q", st)
	}
	// pending + unrelated dead delivery (even endpoint-level) → survives.
	setStatus("pending")
	mark(fmt.Sprintf("https://api.test.dev/objects/%d", ts), "remote 410", true)
	if st := status(); st != "pending" {
		t.Fatalf("pending relay unwound by unrelated inbox traffic: %q", st)
	}
	// pending + own Follow dead-letters — even a 4xx is a refused
	// handshake → failed.
	mark(followID, "remote 403", false)
	if st := status(); st != "failed" {
		t.Fatalf("pending relay not unwound by own refused Follow: %q", st)
	}
}

func TestDBAuthorizedFetchGate(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)
	svc.SetSettings(&settingsStub{kv: map[string]string{"federation.authorized_fetch": "true"}})

	// Unsigned GET → ErrSignatureRequired.
	req := httptest.NewRequest("GET", "https://api.test.dev/federation/actors/alice", nil)
	if err := svc.VerifyFetchSignature(req); err != ErrSignatureRequired {
		t.Fatalf("unsigned fetch: %v", err)
	}

	// Signed GET by a known remote actor → nil.
	pubPEM, privPEM, err := generateActorKeyPair()
	if err != nil {
		t.Fatalf("keypair: %v", err)
	}
	priv, err := parsePrivateKeyPEM(privPEM)
	if err != nil {
		t.Fatalf("priv parse: %v", err)
	}
	signerURL := "https://fetcher.example/actors/fetcher"
	keyID := signerURL + "#main-key"
	if _, err := svc.store.upsertRemoteActor(ctx, gen.UpsertRemoteActorParams{
		ActorUrl:     signerURL,
		Domain:       "fetcher.example",
		InboxUrl:     strPtr("https://fetcher.example/inbox"),
		PublicKeyPem: strPtr(pubPEM),
		PublicKeyID:  strPtr(keyID),
	}); err != nil {
		t.Fatalf("seed signer: %v", err)
	}
	defer func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM remote_actors WHERE actor_url = $1`, signerURL)
	}()

	req2 := httptest.NewRequest("GET", "https://api.test.dev/federation/actors/alice", nil)
	if err := SignRequest(req2, keyID, priv, nil); err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := svc.VerifyFetchSignature(req2); err != nil {
		t.Fatalf("signed fetch rejected: %v", err)
	}

	// Tampered path (re-sign not possible → signature invalid) → required.
	req3 := httptest.NewRequest("GET", "https://api.test.dev/federation/actors/alice", nil)
	if err := SignRequest(req3, keyID, priv, nil); err != nil {
		t.Fatalf("sign: %v", err)
	}
	req3.URL.Path = "/federation/actors/bob"
	if err := svc.VerifyFetchSignature(req3); err != ErrSignatureRequired {
		t.Fatalf("tampered fetch: %v", err)
	}

	// Setting off → unsigned passes.
	svc.SetSettings(&settingsStub{kv: map[string]string{}})
	if err := svc.VerifyFetchSignature(req); err != nil {
		t.Fatalf("disabled mode rejects unsigned: %v", err)
	}
}

func TestDBHashtagEmission(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)
	userID, _ := newFederationUser(t, ctx, d, "active")
	contentID, _ := newFederatedContent(t, ctx, svc, userID)

	var tagID pgtype.UUID
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO tags (name, slug) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("FedTag%d", time.Now().UnixNano()%100000),
		fmt.Sprintf("fedtag-%d", time.Now().UnixNano())).Scan(&tagID); err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	defer func() { _, _ = d.Writer.Exec(context.Background(), `DELETE FROM tags WHERE id = $1`, tagID) }()
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO content_tags (content_id, tag_id) VALUES ($1, $2)`, contentID, tagID); err != nil {
		t.Fatalf("link tag: %v", err)
	}

	doc := map[string]any{"id": "x"}
	svc.decorateTags(ctx, doc, contentID)
	tags, ok := doc["tag"].([]any)
	if !ok || len(tags) != 1 {
		t.Fatalf("tag entries: %v", doc["tag"])
	}
	entry := tags[0].(map[string]any)
	if entry["type"] != "Hashtag" || entry["href"] == "" || entry["name"] == "" {
		t.Errorf("tag entry: %+v", entry)
	}

	// ObjectDocument carries the tag array end-to-end.
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	obj, err := svc.ObjectDocument(ctx, contentID)
	if err != nil {
		t.Fatalf("ObjectDocument: %v", err)
	}
	if _, ok := obj["tag"]; !ok {
		t.Error("object doc missing tag[]")
	}
}

// TestDBGroupTombstoneDropsCaches — a tombstoned group must stop
// resolving on BOTH cached surfaces (doc + WebFinger JRD). The
// webfinger key originally survived: the handle kept resolving up to
// the 5-minute TTL after the actor died.
func TestDBGroupTombstoneDropsCaches(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	vk, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{mr.Addr()}, DisableCache: true})
	if err != nil {
		t.Fatalf("valkey: %v", err)
	}
	t.Cleanup(vk.Close)
	svc.SetValkey(valkeycompat.NewAdapter(vk))

	spaceID, slug := newSpace(t, ctx, d, "public")
	adminID, _ := newFederationUser(t, ctx, d, "active")
	actor, err := svc.ProvisionGroupActor(ctx, spaceID, adminID)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	// Warm both surfaces.
	if _, err := svc.GroupDocument(ctx, slug); err != nil {
		t.Fatalf("doc warm: %v", err)
	}
	if _, err := svc.groupWebfinger(ctx, slug, "test.dev"); err != nil {
		t.Fatalf("webfinger warm: %v", err)
	}
	lower := strings.ToLower(slug)
	for _, k := range []string{"jc:fed:groupdoc:" + lower, "jc:fed:webfinger:!" + lower} {
		if !mr.Exists(k) {
			t.Fatalf("%s not cached after warm", k)
		}
	}

	if err := svc.TombstoneGroupActor(ctx, actor.ID); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	for _, k := range []string{"jc:fed:groupdoc:" + lower, "jc:fed:webfinger:!" + lower} {
		if mr.Exists(k) {
			t.Errorf("%s survived tombstone", k)
		}
	}
}

// TestDBGroupEnvelopeLapseStopsCachedResolution — the envelope gate must
// run live on every request: a space that flips private stops resolving
// immediately even while its cached doc and WebFinger JRD are still warm.
// The caches only elide the doc/JRD build, never the eligibility join.
func TestDBGroupEnvelopeLapseStopsCachedResolution(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	vk, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{mr.Addr()}, DisableCache: true})
	if err != nil {
		t.Fatalf("valkey: %v", err)
	}
	t.Cleanup(vk.Close)
	svc.SetValkey(valkeycompat.NewAdapter(vk))

	spaceID, slug := newSpace(t, ctx, d, "public")
	adminID, _ := newFederationUser(t, ctx, d, "active")
	if _, err := svc.ProvisionGroupActor(ctx, spaceID, adminID); err != nil {
		t.Fatalf("provision: %v", err)
	}
	if _, err := svc.GroupDocument(ctx, slug); err != nil {
		t.Fatalf("doc warm: %v", err)
	}
	if _, err := svc.groupWebfinger(ctx, slug, "test.dev"); err != nil {
		t.Fatalf("webfinger warm: %v", err)
	}
	lower := strings.ToLower(slug)
	for _, k := range []string{"jc:fed:groupdoc:" + lower, "jc:fed:webfinger:!" + lower} {
		if !mr.Exists(k) {
			t.Fatalf("%s not cached after warm", k)
		}
	}

	// The space leaves the envelope with no cache invalidation wired.
	if _, err := d.Writer.Exec(ctx,
		`UPDATE spaces SET visibility = 'private' WHERE id = $1`, spaceID); err != nil {
		t.Fatalf("flip private: %v", err)
	}

	// Both surfaces must gate live even though both keys remain warm.
	for _, k := range []string{"jc:fed:groupdoc:" + lower, "jc:fed:webfinger:!" + lower} {
		if !mr.Exists(k) {
			t.Fatalf("%s should still be cached — the fix is gating order, not invalidation", k)
		}
	}
	if _, err := svc.GroupDocument(ctx, slug); err != ErrNotFound {
		t.Fatalf("doc after envelope lapse: %v", err)
	}
	if _, err := svc.groupWebfinger(ctx, slug, "test.dev"); err != ErrNotFound {
		t.Fatalf("webfinger after envelope lapse: %v", err)
	}
}

// TestDBGroupAnnounceEnvelopeLapse — a space that leaves the
// member-created/public/active envelope stops resolving, so its group
// actor must stop boosting: fanout must not ledger an Announce whose
// actor URL would 404 on the remote's fetch. The gate runs live via
// groupDocRow — nothing transitions federated_actors.status on a space
// mutation.
func TestDBGroupAnnounceEnvelopeLapse(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)
	defer func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM federation_deliveries WHERE activity_type = 'Announce'`)
	}()

	spaceID, _ := newSpace(t, ctx, d, "public")
	adminID, _ := newFederationUser(t, ctx, d, "active")
	group, err := svc.ProvisionGroupActor(ctx, spaceID, adminID)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	remote, _ := newRemoteActor(t, ctx, svc, "grp-lapse.example")
	if _, err := svc.store.write.UpsertFederationFollow(ctx, gen.UpsertFederationFollowParams{
		LocalActorID: group.ID, RemoteActorID: remote.ID, Status: "accepted",
	}); err != nil {
		t.Fatalf("group edge: %v", err)
	}

	author, _ := newFederationUser(t, ctx, d, "active")
	if _, err := svc.EnableFederation(ctx, author); err != nil {
		t.Fatalf("enable author: %v", err)
	}
	var contentID pgtype.UUID
	if err := d.Writer.QueryRow(ctx, `
		INSERT INTO contents (author_id, space_id, content_type, title, slug, body, body_html,
		                      content_hash, status, visibility, published_at)
		VALUES ($1, $2, 'story', 'Lapse post', $3, 'hi', '<p>hi</p>', $4, 'published', 'public', now())
		RETURNING id`, author, spaceID,
		fmt.Sprintf("lapse-%d", time.Now().UnixNano()),
		fmt.Sprintf("hash-lapse-%d", time.Now().UnixNano())).Scan(&contentID); err != nil {
		t.Fatalf("insert content: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM contents WHERE id = $1`, contentID)
	})

	// Space leaves the envelope — no actor-status mutation fires.
	if _, err := d.Writer.Exec(ctx,
		`UPDATE spaces SET visibility = 'private' WHERE id = $1`, spaceID); err != nil {
		t.Fatalf("flip private: %v", err)
	}
	if err := svc.Fanout(ctx, contentID); err != nil {
		t.Fatalf("Fanout: %v", err)
	}
	var n int
	if err := d.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM federation_deliveries WHERE local_actor_id = $1 AND activity_type = 'Announce'`,
		group.ID).Scan(&n); err != nil {
		t.Fatalf("announce count: %v", err)
	}
	if n != 0 {
		t.Errorf("group Announce ledgered for an envelope-lapsed space (%d rows)", n)
	}
}

// TestDBListRelaysQFilter — the admin DataTable ?q= contract added in
// pass 26: ILIKE on actor_url/domain.
func TestDBListRelaysQFilter(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)

	a, _ := newRemoteActor(t, ctx, svc, "relay-q-a.example")
	b, _ := newRemoteActor(t, ctx, svc, "other-host.test")
	_ = newRelayRow(t, ctx, svc, a.ActorUrl, "accepted")
	_ = newRelayRow(t, ctx, svc, b.ActorUrl, "pending")

	rows, total, err := svc.ListRelays(ctx, "other-host", "", 25, 0)
	if err != nil {
		t.Fatalf("ListRelays q: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].Domain != "other-host.test" {
		t.Fatalf("q filter: total=%d rows=%v", total, rows)
	}
	// Empty q returns at least our two (other tests may seed more).
	if _, total, err := svc.ListRelays(ctx, "", "", 25, 0); err != nil || total < 2 {
		t.Fatalf("unfiltered: total=%d err=%v", total, err)
	}
	// status + q compose.
	if _, total, err := svc.ListRelays(ctx, "", "accepted", 25, 0); err != nil || total != 1 {
		t.Fatalf("status filter: total=%d err=%v", total, err)
	}
}

// TestDBListGroupActorsQ — the group list's ?q= contract: ILIKE on
// space slug/name or actor URL.
func TestDBListGroupActorsQ(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)
	adminID, _ := newFederationUser(t, ctx, d, "active")

	idA, slugA := newSpace(t, ctx, d, "public")
	if _, err := svc.ProvisionGroupActor(ctx, idA, adminID); err != nil {
		t.Fatalf("provision A: %v", err)
	}
	idB, _ := newSpace(t, ctx, d, "public")
	if _, err := svc.ProvisionGroupActor(ctx, idB, adminID); err != nil {
		t.Fatalf("provision B: %v", err)
	}

	rows, total, err := svc.ListGroupActors(ctx, slugA, 25, 0)
	if err != nil {
		t.Fatalf("ListGroupActors q: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].SpaceSlug != slugA {
		t.Fatalf("q filter: total=%d rows=%v", total, rows)
	}
	if _, total, err := svc.ListGroupActors(ctx, "", 25, 0); err != nil || total < 2 {
		t.Fatalf("unfiltered: total=%d err=%v", total, err)
	}
}

// Pass 28: outbox collections must not enumerate content whose author has
// no active federated actor — ObjectDocument 404s those fetches, so a
// listed item would dangle for remotes.
func TestDBOutboxAuthorGate(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)

	spaceID, slug := newSpace(t, ctx, d, "public")
	adminID, _ := newFederationUser(t, ctx, d, "active")
	group, err := svc.ProvisionGroupActor(ctx, spaceID, adminID)
	if err != nil {
		t.Fatalf("provision group: %v", err)
	}

	// A member with NO federated actor posting public content in the space.
	author, _ := newFederationUser(t, ctx, d, "active")
	var contentID pgtype.UUID
	if err := d.Writer.QueryRow(ctx, `
		INSERT INTO contents (author_id, space_id, content_type, title, slug, body, body_html,
		                      content_hash, status, visibility, published_at)
		VALUES ($1, $2, 'story', 'Unfed post', $3, 'hi', '<p>hi</p>', $4, 'published', 'public', now())
		RETURNING id`, author, spaceID,
		fmt.Sprintf("unfed-%d", time.Now().UnixNano()),
		fmt.Sprintf("hash-unfed-%d", time.Now().UnixNano())).Scan(&contentID); err != nil {
		t.Fatalf("insert content: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM contents WHERE id = $1`, contentID)
	})

	// The object itself must 404 (author not federated) — and therefore
	// neither collection may list it.
	if _, err := svc.ObjectDocument(ctx, contentID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("object doc before federation: %v", err)
	}
	gOut, err := svc.GroupOutbox(ctx, slug, 0)
	if err != nil {
		t.Fatalf("group outbox: %v", err)
	}
	if got := gOut["totalItems"].(int64); got != 0 {
		t.Fatalf("group outbox listed unfederated-author content: %d", got)
	}
	instBefore, err := svc.InstanceOutbox(ctx, 0)
	if err != nil {
		t.Fatalf("instance outbox: %v", err)
	}
	beforeTotal := instBefore["totalItems"].(int64)

	// Enable federation → the content becomes resolvable AND enumerable.
	if _, err := svc.EnableFederation(ctx, author); err != nil {
		t.Fatalf("enable federation: %v", err)
	}
	if _, err := svc.ObjectDocument(ctx, contentID); err != nil {
		t.Fatalf("object doc after federation: %v", err)
	}
	gOut2, err := svc.GroupOutbox(ctx, slug, 0)
	if err != nil {
		t.Fatalf("group outbox after: %v", err)
	}
	if got := gOut2["totalItems"].(int64); got != 1 {
		t.Fatalf("group outbox after federation: want 1, got %d", got)
	}
	instAfter, err := svc.InstanceOutbox(ctx, 0)
	if err != nil {
		t.Fatalf("instance outbox after: %v", err)
	}
	if got := instAfter["totalItems"].(int64); got != beforeTotal+1 {
		t.Fatalf("instance outbox delta: want +1, before=%d after=%d", beforeTotal, got)
	}

	// A disabled actor drops the item back out — the gate is status-aware.
	if _, err := svc.DisableFederation(ctx, author); err != nil {
		t.Fatalf("disable federation: %v", err)
	}
	gOut3, err := svc.GroupOutbox(ctx, slug, 0)
	if err != nil {
		t.Fatalf("group outbox disabled: %v", err)
	}
	if got := gOut3["totalItems"].(int64); got != 0 {
		t.Fatalf("group outbox after disable: want 0, got %d", got)
	}
	_ = group
}
