// Ref: japaconnect-engineering/sections/127-atproto-native.md (A6-1 lifecycle)
// Tombstone/erasure core: #tombstone frames, the shared TombstoneAccount path,
// admin lifecycle ops, and remote-actor deactivation propagation.
package atproto

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	gen "github.com/jedafe/japa/internal/db/gen"
	cbg "github.com/whyrusleeping/cbor-gen"
)

// accountEvent builds a Jetstream account event for did.
func accountEvent(did string, active bool) *jetstreamEvent {
	ev := &jetstreamEvent{DID: did, Kind: "account"}
	ev.Account = &struct {
		Active bool   `json:"active"`
		DID    string `json:"did"`
		Status string `json:"status"`
		Seq    int64  `json:"seq"`
		Time   string `json:"time"`
	}{Active: active, DID: did}
	return ev
}

// tombstoneEvents filters a did's event rows down to the tombstone kind.
func tombstoneEvents(events []gen.AtprotoRepoEvent) []gen.AtprotoRepoEvent {
	out := make([]gen.AtprotoRepoEvent, 0, 1)
	for _, ev := range events {
		if ev.Kind == EventKindTombstone {
			out = append(out, ev)
		}
	}
	return out
}

// TestTombstoneFrameRoundTrip — a stored tombstone event marshals into the
// two-part CBOR frame relays expect: {op:1,t:"#tombstone"} + {seq,did,time}.
func TestTombstoneFrameRoundTrip(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"did": "did:web:api.test.dev:atproto:u:abc"})
	msg, err := frameBytes(gen.AtprotoRepoEvent{
		Seq:     42,
		Did:     "did:web:api.test.dev:atproto:u:abc",
		Kind:    EventKindTombstone,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("frameBytes: %v", err)
	}
	op, typ, cr := decodeFrameHeader(t, msg)
	if op != frameOpMessage || typ != "#tombstone" {
		t.Fatalf("frame header op=%d t=%q — want {op:1,t:#tombstone}", op, typ)
	}
	// Body: a 3-key map {seq:int, did:text, time:text} — decode generically;
	// indigo ships no tombstone type to UnmarshalCBOR against.
	maj, extra, err := cr.ReadHeader()
	if err != nil || maj != cbg.MajMap || extra != 3 {
		t.Fatalf("tombstone body: maj=%d extra=%d err=%v — want 3-key map", maj, extra, err)
	}
	got := map[string]any{}
	for i := uint64(0); i < extra; i++ {
		kmaj, klen, err := cr.ReadHeader()
		if err != nil || kmaj != cbg.MajTextString {
			t.Fatalf("body key: maj=%d err=%v", kmaj, err)
		}
		kbuf := make([]byte, klen)
		if _, err := io.ReadFull(cr, kbuf); err != nil {
			t.Fatalf("body key read: %v", err)
		}
		vmaj, vlen, err := cr.ReadHeader()
		if err != nil {
			t.Fatalf("body value: %v", err)
		}
		switch vmaj {
		case cbg.MajTextString:
			vbuf := make([]byte, vlen)
			if _, err := io.ReadFull(cr, vbuf); err != nil {
				t.Fatalf("body text read: %v", err)
			}
			got[string(kbuf)] = string(vbuf)
		case cbg.MajUnsignedInt:
			got[string(kbuf)] = int64(vlen)
		case cbg.MajNegativeInt:
			got[string(kbuf)] = -int64(vlen) - 1
		default:
			t.Fatalf("body value %q: unexpected maj %d", kbuf, vmaj)
		}
	}
	if got["seq"] != int64(42) || got["did"] != "did:web:api.test.dev:atproto:u:abc" {
		t.Fatalf("tombstone body = %v", got)
	}
	if _, ok := got["time"].(string); !ok || got["time"] == "" {
		t.Fatalf("tombstone body missing time: %v", got)
	}
}

// TestDBEraseAccountEmitsTombstone — GDPR erasure ends the account as
// tombstone with the #account{deleted} → #tombstone tail in seq order,
// records retracted, and the member endpoints unable to resurrect it.
func TestDBEraseAccountEmitsTombstone(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	svc.SetValkey(startTestCompat(t))

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")
	if _, err := svc.PublishContent(ctx, contentID); err != nil {
		t.Fatalf("PublishContent: %v", err)
	}

	if err := svc.EraseAccount(ctx, userID); err != nil {
		t.Fatalf("EraseAccount: %v", err)
	}

	st := atprotoAccountFor(t, ctx, d, userID)
	if st.Status != AccountStatusTombstone {
		t.Fatalf("status = %q, want tombstone", st.Status)
	}
	var recs int
	if err := d.Writer.QueryRow(ctx,
		`SELECT count(*) FROM atproto_records WHERE account_id = $1`, acct.ID).Scan(&recs); err != nil {
		t.Fatalf("records count: %v", err)
	}
	if recs != 0 {
		t.Fatalf("records left indexed: %d", recs)
	}

	events := repoEvents(t, ctx, d, acct.Did)
	tombs := tombstoneEvents(events)
	if len(tombs) != 1 {
		t.Fatalf("tombstone events = %d, want 1 (all: %v)", len(tombs), events)
	}
	// The tombstone frame is the stream's last word: it tails the
	// #account{deleted} frame in seq order.
	if events[len(events)-1].Kind != EventKindTombstone {
		t.Fatalf("last event kind = %q, want tombstone", events[len(events)-1].Kind)
	}
	prev := events[len(events)-2]
	if prev.Kind != EventKindAccount {
		t.Fatalf("penultimate event kind = %q, want account (all: %v)", prev.Kind, events)
	}
	var acctFrame struct {
		Active bool    `json:"active"`
		Status *string `json:"status"`
	}
	if err := json.Unmarshal(prev.Payload, &acctFrame); err != nil {
		t.Fatalf("account payload: %v", err)
	}
	if acctFrame.Active || acctFrame.Status == nil || *acctFrame.Status != "deleted" {
		t.Fatalf("account frame = %+v, want {active:false,status:deleted}", acctFrame)
	}
	if tombs[0].Seq <= prev.Seq {
		t.Fatalf("tombstone seq %d not after account seq %d", tombs[0].Seq, prev.Seq)
	}
	// The stored row must marshal into a decodable #tombstone frame.
	msg, err := frameBytes(tombs[0])
	if err != nil {
		t.Fatalf("frameBytes: %v", err)
	}
	op, typ, _ := decodeFrameHeader(t, msg)
	if op != frameOpMessage || typ != "#tombstone" {
		t.Fatalf("frame = op %d t %q", op, typ)
	}
	var body struct {
		Did string `json:"did"`
	}
	if err := json.Unmarshal(tombs[0].Payload, &body); err != nil || body.Did != acct.Did {
		t.Fatalf("tombstone payload = %s err=%v", tombs[0].Payload, err)
	}

	// Terminal — member endpoints never resurrect.
	if _, err := svc.EnableAccount(ctx, userID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("re-enable tombstoned: got %v, want ErrNotFound", err)
	}
}

// TestDBAdminLifecycleOps — the atproto.accounts.manage matrix: force-
// disable holds past member self re-enable and lifecycle sync, reason
// escalation doesn't double-emit, reactivate lifts any non-tombstone
// freeze, and tombstone stays terminal under every op.
func TestDBAdminLifecycleOps(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	svc.SetValkey(startTestCompat(t))

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	before := len(repoEvents(t, ctx, d, acct.Did))

	// Force-disable: status + reason + the deactivated frame.
	st, err := svc.AdminDisableAccount(ctx, acct.ID)
	if err != nil {
		t.Fatalf("AdminDisableAccount: %v", err)
	}
	if st.Status != AccountStatusDisabled {
		t.Fatalf("admin disable status = %q", st.Status)
	}
	row := atprotoAccountFor(t, ctx, d, userID)
	if row.DisabledReason == nil || *row.DisabledReason != DisableReasonAdmin {
		t.Fatalf("disabled_reason = %v, want admin", row.DisabledReason)
	}
	events := repoEvents(t, ctx, d, acct.Did)
	if len(events) != before+1 || events[len(events)-1].Kind != EventKindAccount {
		t.Fatalf("events after admin disable = %d, want %d with account tail", len(events), before+1)
	}
	var deact struct {
		Active bool    `json:"active"`
		Status *string `json:"status"`
	}
	if err := json.Unmarshal(events[len(events)-1].Payload, &deact); err != nil {
		t.Fatalf("account payload: %v", err)
	}
	if deact.Active || deact.Status == nil || *deact.Status != "deactivated" {
		t.Fatalf("account frame = %+v, want deactivated", deact)
	}

	// Member self re-enable refuses; lifecycle sync ignores 'admin' rows.
	if _, err := svc.EnableAccount(ctx, userID); !errors.Is(err, ErrAdminDisabled) {
		t.Fatalf("member re-enable admin-disabled: got %v, want ErrAdminDisabled", err)
	}
	if _, err := svc.SyncUserLifecycle(ctx, userID); err != nil {
		t.Fatalf("SyncUserLifecycle: %v", err)
	}
	if row := atprotoAccountFor(t, ctx, d, userID); row.Status != AccountStatusDisabled {
		t.Fatalf("lifecycle reactivated admin row to %q", row.Status)
	}

	// Admin reactivate lifts it: active, reason cleared, activation frame.
	st, err = svc.AdminReactivateAccount(ctx, acct.ID)
	if err != nil {
		t.Fatalf("AdminReactivateAccount: %v", err)
	}
	if st.Status != AccountStatusActive {
		t.Fatalf("admin reactivate status = %q", st.Status)
	}
	if row := atprotoAccountFor(t, ctx, d, userID); row.DisabledReason != nil {
		t.Fatalf("reason after reactivate = %v, want NULL", *row.DisabledReason)
	}
	events = repoEvents(t, ctx, d, acct.Did)
	var act struct {
		Active bool `json:"active"`
	}
	if err := json.Unmarshal(events[len(events)-1].Payload, &act); err != nil || !act.Active {
		t.Fatalf("reactivate frame = %s", events[len(events)-1].Payload)
	}

	// Reason escalation: admin-disabling a member-disabled row flips the
	// reason WITHOUT a duplicate deactivated frame (state didn't change).
	if _, err := svc.DisableAccount(ctx, userID); err != nil {
		t.Fatalf("DisableAccount: %v", err)
	}
	before = len(repoEvents(t, ctx, d, acct.Did))
	if _, err := svc.AdminDisableAccount(ctx, acct.ID); err != nil {
		t.Fatalf("AdminDisableAccount escalate: %v", err)
	}
	row = atprotoAccountFor(t, ctx, d, userID)
	if row.DisabledReason == nil || *row.DisabledReason != DisableReasonAdmin {
		t.Fatalf("escalated reason = %v, want admin", row.DisabledReason)
	}
	if n := len(repoEvents(t, ctx, d, acct.Did)); n != before {
		t.Fatalf("escalation emitted %d new events — expected none", n-before)
	}
	if _, err := svc.AdminReactivateAccount(ctx, acct.ID); err != nil {
		t.Fatalf("reactivate after escalation: %v", err)
	}

	// Tombstone: terminal under every op.
	if _, err := svc.AdminTombstoneAccount(ctx, acct.ID); err != nil {
		t.Fatalf("AdminTombstoneAccount: %v", err)
	}
	if row := atprotoAccountFor(t, ctx, d, userID); row.Status != AccountStatusTombstone {
		t.Fatalf("status = %q, want tombstone", row.Status)
	}
	if tombs := tombstoneEvents(repoEvents(t, ctx, d, acct.Did)); len(tombs) != 1 {
		t.Fatalf("tombstone events = %d, want 1", len(tombs))
	}
	if _, err := svc.AdminReactivateAccount(ctx, acct.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("admin reactivate tombstoned: got %v, want ErrNotFound", err)
	}
	if _, err := svc.AdminDisableAccount(ctx, acct.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("admin disable tombstoned: got %v, want ErrNotFound", err)
	}
	if _, err := svc.EnableAccount(ctx, userID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("member enable tombstoned: got %v, want ErrNotFound", err)
	}
	// Idempotent — a second tombstone is a clean no-op.
	if _, err := svc.AdminTombstoneAccount(ctx, acct.ID); err != nil {
		t.Fatalf("re-tombstone: %v", err)
	}
}

// TestDBRemoteActorDeactivationPropagates — Jetstream account{active:false}
// tombstones the actor's remote comments, unwinds the counters their
// creates bumped, and undoes their likes/follows; active:true clears the
// flag without restoring anything.
func TestDBRemoteActorDeactivationPropagates(t *testing.T) {
	ctx := context.Background()
	svc, acct, contentID, uri := a5Fixture(t, ctx)
	svc.SetNotifier(&captureNotifier{})

	remoteDID := fmt.Sprintf("did:plc:prop%d", time.Now().UnixNano())
	// Parent + nested remote reply (same actor — the parent's reply_count
	// unwind must skip the tombstoned parent itself, not decrement it).
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "prop1", 900, colFeedPost, map[string]any{
		"text":  "parent reply",
		"reply": map[string]any{"parent": map[string]any{"uri": uri}, "root": map[string]any{"uri": uri}},
	}))
	parentURI := "at://" + remoteDID + "/app.bsky.feed.post/prop1"
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "prop2", 901, colFeedPost, map[string]any{
		"text":  "nested reply",
		"reply": map[string]any{"parent": map[string]any{"uri": parentURI}, "root": map[string]any{"uri": uri}},
	}))
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "proplike", 902, colFeedLike, map[string]any{
		"subject": map[string]any{"uri": uri, "cid": "bafy-y"},
	}))
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "propfollow", 903, colFollow, map[string]any{
		"subject": acct.Did,
	}))

	d := testDB(t)
	var cc int
	if err := d.Writer.QueryRow(ctx, `SELECT comment_count FROM contents WHERE id = $1`, contentID).Scan(&cc); err != nil || cc != 2 {
		t.Fatalf("pre-deactivation comment_count=%d err=%v", cc, err)
	}

	svc.handleAccountEvent(ctx, accountEvent(remoteDID, false))

	var deactAt *time.Time
	if err := d.Writer.QueryRow(ctx,
		`SELECT deactivated_at FROM atproto_remote_actors WHERE did = $1`, remoteDID).Scan(&deactAt); err != nil {
		t.Fatalf("actor lookup: %v", err)
	}
	if deactAt == nil {
		t.Fatalf("deactivated_at not set")
	}
	var live, total int
	if err := d.Writer.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE deleted_at IS NULL), count(*)
		 FROM comments WHERE remote_atproto_actor_id = (SELECT id FROM atproto_remote_actors WHERE did = $1)`,
		remoteDID).Scan(&live, &total); err != nil {
		t.Fatalf("comments count: %v", err)
	}
	if total != 2 || live != 0 {
		t.Fatalf("comments total=%d live=%d — want 2 tombstoned", total, live)
	}
	if err := d.Writer.QueryRow(ctx, `SELECT comment_count FROM contents WHERE id = $1`, contentID).Scan(&cc); err != nil || cc != 0 {
		t.Fatalf("post-deactivation comment_count=%d err=%v", cc, err)
	}
	// The parent's own reply_count isn't decremented — it's tombstoned; the
	// single-delete path's deleted_at guard has the same shape.
	var likeUndone, followUndone *time.Time
	_ = d.Writer.QueryRow(ctx,
		`SELECT undone_at FROM atproto_remote_likes WHERE like_uri = $1`,
		"at://"+remoteDID+"/app.bsky.feed.like/proplike").Scan(&likeUndone)
	_ = d.Writer.QueryRow(ctx,
		`SELECT undone_at FROM atproto_remote_follows WHERE follow_uri = $1`,
		"at://"+remoteDID+"/app.bsky.graph.follow/propfollow").Scan(&followUndone)
	if likeUndone == nil || followUndone == nil {
		t.Fatalf("reactions not undone: like=%v follow=%v", likeUndone, followUndone)
	}

	// Reactivation clears the flag only — tombstoned comments and undone
	// reactions stay down; the stream re-ingests future creates naturally.
	svc.handleAccountEvent(ctx, accountEvent(remoteDID, true))
	if err := d.Writer.QueryRow(ctx,
		`SELECT deactivated_at FROM atproto_remote_actors WHERE did = $1`, remoteDID).Scan(&deactAt); err != nil {
		t.Fatalf("actor relookup: %v", err)
	}
	if deactAt != nil {
		t.Fatalf("deactivated_at not cleared")
	}
	if err := d.Writer.QueryRow(ctx,
		`SELECT count(*) FROM comments WHERE remote_atproto_actor_id = (SELECT id FROM atproto_remote_actors WHERE did = $1) AND deleted_at IS NULL`,
		remoteDID).Scan(&live); err != nil || live != 0 {
		t.Fatalf("reactivation restored %d comments err=%v — want none", live, err)
	}
}

// TestDBRemoteActorEventUntrackedNoop — account events for an untracked DID
// mint nothing: no actor row, no comment writes.
func TestDBRemoteActorEventUntrackedNoop(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := a5Fixture(t, ctx)
	d := testDB(t)

	untracked := fmt.Sprintf("did:plc:untrack%d", time.Now().UnixNano())
	svc.handleAccountEvent(ctx, accountEvent(untracked, false))
	svc.handleAccountEvent(ctx, accountEvent(untracked, true))

	var n int
	if err := d.Writer.QueryRow(ctx,
		`SELECT count(*) FROM atproto_remote_actors WHERE did = $1`, untracked).Scan(&n); err != nil {
		t.Fatalf("actor count: %v", err)
	}
	if n != 0 {
		t.Fatalf("account event minted actor row for untracked DID")
	}
}

// TestDBMemberOptOutReversible — DELETE /me/atproto is a reversible
// disable (reason 'member'), never a tombstone; POST re-enables it.
func TestDBMemberOptOutReversible(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	svc.SetValkey(startTestCompat(t))

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	st, err := svc.DisableAccount(ctx, userID)
	if err != nil {
		t.Fatalf("DisableAccount: %v", err)
	}
	if st.Status != AccountStatusDisabled {
		t.Fatalf("opt-out status = %q, want disabled (reversible)", st.Status)
	}
	row := atprotoAccountFor(t, ctx, d, userID)
	if row.DisabledReason == nil || *row.DisabledReason != DisableReasonMember {
		t.Fatalf("opt-out reason = %v, want member", row.DisabledReason)
	}
	st, err = svc.EnableAccount(ctx, userID)
	if err != nil {
		t.Fatalf("member re-enable: %v", err)
	}
	if st.Status != AccountStatusActive {
		t.Fatalf("re-enabled status = %q", st.Status)
	}
}
