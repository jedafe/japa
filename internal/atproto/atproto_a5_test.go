// Ref: japaconnect-engineering/sections/127-atproto-native.md (A5 — Jetstream inbound)
// Inbound normalization tests: dispatch maps commit events into comments /
// remote likes / remote follows + notifications + ledger rows, with dedup on
// redelivery.
package atproto

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

type captureNotifier struct {
	calls []struct {
		Recipient  pgtype.UUID
		Type       string
		ActorType  string
		ActorID    pgtype.UUID
		SubjectID  pgtype.UUID
		Title      string
		Body       string
		ActionURL  string
	}
}

func (c *captureNotifier) CreateNotification(_ context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, _ string, subjectID pgtype.UUID, title, body, actionURL string) error {
	c.calls = append(c.calls, struct {
		Recipient pgtype.UUID
		Type      string
		ActorType string
		ActorID   pgtype.UUID
		SubjectID pgtype.UUID
		Title     string
		Body      string
		ActionURL string
	}{recipientID, nType, actorType, actorID, subjectID, title, body, actionURL})
	return nil
}

// a5Fixture wires a member account + one projected feed.post record and
// returns the pieces the dispatch tests need.
func a5Fixture(t *testing.T, ctx context.Context) (*Service, gen.AtprotoAccount, pgtype.UUID, string) {
	t.Helper()
	d := testDB(t)
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	svc.SetValkey(startTestCompat(t))
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)

	contentID := newAtprotoContent(t, ctx, d, userID, "post", "published", "public")
	rkey := "a5post" + fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	uri := "at://" + acct.Did + "/app.bsky.feed.post/" + rkey
	if _, err := gen.New(d.Writer).UpsertAtprotoRecord(ctx, gen.UpsertAtprotoRecordParams{
		Uri:        uri,
		AccountID:  acct.ID,
		Collection: "app.bsky.feed.post",
		Rkey:       rkey,
		Cid:        "bafy-a5",
		ContentID:  contentID,
	}); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	return svc, acct, contentID, uri
}

func commitEvent(did, rkey string, seq int64, record map[string]any) *jetstreamEvent {
	ev := &jetstreamEvent{DID: did, TimeUS: seq, Kind: "commit"}
	ev.Commit = &struct {
		Rev        string         `json:"rev"`
		Operation  string         `json:"operation"`
		Collection string         `json:"collection"`
		Rkey       string         `json:"rkey"`
		Record     map[string]any `json:"record"`
		CID        string         `json:"cid"`
	}{Operation: "create", Rkey: rkey, Record: record}
	return ev
}

// TestDBInboundReplyCreatesComment — a remote reply on a member post lands as
// a remote-authored comment + post_reply notification + processed ledger row.
func TestDBInboundReplyCreatesComment(t *testing.T) {
	ctx := context.Background()
	svc, acct, contentID, uri := a5Fixture(t, ctx)
	notifier := &captureNotifier{}
	svc.SetNotifier(notifier)

	remoteDID := fmt.Sprintf("did:plc:ra%d", time.Now().UnixNano())
	ev := commitEvent(remoteDID, "reply1", 100, map[string]any{
		"text": "nice post from bluesky",
		"reply": map[string]any{
			"parent": map[string]any{"uri": uri, "cid": "bafy-x"},
			"root":   map[string]any{"uri": uri, "cid": "bafy-x"},
		},
	})
	ev.Commit.Collection = colFeedPost
	svc.handleCommit(ctx, ev)

	d := testDB(t)
	var body []byte
	var remoteActor pgtype.UUID
	err := d.Writer.QueryRow(ctx,
		`SELECT body, remote_atproto_actor_id FROM comments
		 WHERE remote_uri = $1`, "at://"+remoteDID+"/app.bsky.feed.post/reply1").Scan(&body, &remoteActor)
	if err != nil {
		t.Fatalf("remote comment missing: %v", err)
	}
	if !strings.Contains(string(body), "nice post from bluesky") || !remoteActor.Valid {
		t.Fatalf("bad comment body=%q actor=%v", body, remoteActor.Valid)
	}
	var contentRef pgtype.UUID
	_ = d.Writer.QueryRow(ctx, `SELECT content_id FROM comments WHERE remote_uri = $1`,
		"at://"+remoteDID+"/app.bsky.feed.post/reply1").Scan(&contentRef)
	if contentRef != contentID {
		t.Fatalf("comment on wrong content")
	}

	var status string
	_ = d.Writer.QueryRow(ctx, `SELECT status FROM atproto_inbound WHERE event_uri = $1`,
		"at://"+remoteDID+"/app.bsky.feed.post/reply1").Scan(&status)
	if status != "processed" {
		t.Fatalf("inbound status=%s", status)
	}

	if len(notifier.calls) != 1 || notifier.calls[0].Type != "post_reply" || notifier.calls[0].ActorType != "atproto_actor" {
		t.Fatalf("notification calls=%+v", notifier.calls)
	}
	_ = acct
}

// TestDBInboundLikeUndo — like create lands a remote_likes row + reaction
// notification; the matching delete op flips undone_at.
func TestDBInboundLikeUndo(t *testing.T) {
	ctx := context.Background()
	svc, _, contentID, uri := a5Fixture(t, ctx)
	svc.SetNotifier(&captureNotifier{})

	remoteDID := fmt.Sprintf("did:plc:lk%d", time.Now().UnixNano())
	likeURI := "at://" + remoteDID + "/app.bsky.feed.like/like1"
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "like1", 200, colFeedLike, map[string]any{
		"subject": map[string]any{"uri": uri, "cid": "bafy-y"},
	}))

	d := testDB(t)
	var gotContent pgtype.UUID
	var undone *time.Time
	err := d.Writer.QueryRow(ctx,
		`SELECT content_id, undone_at FROM atproto_remote_likes WHERE like_uri = $1`, likeURI).Scan(&gotContent, &undone)
	if err != nil {
		t.Fatalf("remote like missing: %v", err)
	}
	if gotContent != contentID || undone != nil {
		t.Fatalf("like row wrong: content=%v undone=%v", gotContent, undone)
	}

	del := commitEventWithURI(remoteDID, "like1", 201, colFeedLike, nil)
	del.Commit.Operation = "delete"
	svc.handleCommit(ctx, del)
	_ = d.Writer.QueryRow(ctx, `SELECT undone_at FROM atproto_remote_likes WHERE like_uri = $1`, likeURI).Scan(&undone)
	if undone == nil {
		t.Fatalf("like not undone")
	}
}

// TestDBInboundFollowUndo — follow create on a member DID lands a
// remote_follows row + new_follower notification; delete undoes it.
func TestDBInboundFollowUndo(t *testing.T) {
	ctx := context.Background()
	svc, acct, _, _ := a5Fixture(t, ctx)
	notifier := &captureNotifier{}
	svc.SetNotifier(notifier)

	remoteDID := fmt.Sprintf("did:plc:fw%d", time.Now().UnixNano())
	followURI := "at://" + remoteDID + "/app.bsky.graph.follow/f1"
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "f1", 300, colFollow, map[string]any{
		"subject": acct.Did,
	}))

	d := testDB(t)
	var acctID pgtype.UUID
	var undone *time.Time
	err := d.Writer.QueryRow(ctx,
		`SELECT account_id, undone_at FROM atproto_remote_follows WHERE follow_uri = $1`, followURI).Scan(&acctID, &undone)
	if err != nil {
		t.Fatalf("remote follow missing: %v", err)
	}
	if acctID != acct.ID || undone != nil {
		t.Fatalf("follow row wrong")
	}
	if len(notifier.calls) != 1 || notifier.calls[0].Type != "new_follower" {
		t.Fatalf("notify calls=%+v", notifier.calls)
	}

	del := commitEventWithURI(remoteDID, "f1", 301, colFollow, nil)
	del.Commit.Operation = "delete"
	svc.handleCommit(ctx, del)
	_ = d.Writer.QueryRow(ctx, `SELECT undone_at FROM atproto_remote_follows WHERE follow_uri = $1`, followURI).Scan(&undone)
	if undone == nil {
		t.Fatalf("follow not undone")
	}
}

// TestDBInboundDedupReplay — the same event redelivered is a no-op: one
// ledger row, one comment, one notification.
func TestDBInboundDedupReplay(t *testing.T) {
	ctx := context.Background()
	svc, _, _, uri := a5Fixture(t, ctx)
	notifier := &captureNotifier{}
	svc.SetNotifier(notifier)

	remoteDID := fmt.Sprintf("did:plc:dp%d", time.Now().UnixNano())
	ev := commitEventWithURI(remoteDID, "r1", 400, colFeedPost, map[string]any{
		"text":  "dup reply",
		"reply": map[string]any{"parent": map[string]any{"uri": uri}},
	})
	svc.handleCommit(ctx, ev)
	svc.handleCommit(ctx, ev)

	d := testDB(t)
	var n int
	_ = d.Writer.QueryRow(ctx, `SELECT COUNT(*) FROM atproto_inbound WHERE event_uri = $1`,
		"at://"+remoteDID+"/app.bsky.feed.post/r1").Scan(&n)
	if n != 1 {
		t.Fatalf("ledger rows=%d", n)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("dedup failed — notify calls=%d", len(notifier.calls))
	}
}

// TestDBInboundIgnoresNonMember — events about non-member URIs/DIDs are
// ledgered as ignored, not processed.
func TestDBInboundIgnoresNonMember(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := a5Fixture(t, ctx)
	svc.SetNotifier(&captureNotifier{})

	remoteDID := fmt.Sprintf("did:plc:os%d", time.Now().UnixNano())
	// Reply to a foreign post — must be ignored.
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "p1", 500, colFeedPost, map[string]any{
		"text":  "reply to someone else",
		"reply": map[string]any{"parent": map[string]any{"uri": "at://did:plc:stranger/app.bsky.feed.post/x"}},
	}))
	// Follow of a foreign DID — ignored.
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "f1", 501, colFollow, map[string]any{
		"subject": "did:plc:somebodyelse",
	}))
	// Top-level post with no reply object — ignored.
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "p2", 502, colFeedPost, map[string]any{
		"text": "just a post",
	}))

	d := testDB(t)
	var statuses []string
	rows, err := d.Writer.Query(ctx, `SELECT status FROM atproto_inbound WHERE did = $1 ORDER BY event_seq`, remoteDID)
	if err != nil {
		t.Fatalf("query inbound: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		statuses = append(statuses, s)
	}
	if len(statuses) != 3 {
		t.Fatalf("statuses=%v", statuses)
	}
	for _, s := range statuses {
		if s != "ignored" {
			t.Fatalf("expected ignored, got %v", statuses)
		}
	}
}

// TestDBInboundRemoteCommentDelete — a remote delete tombstones the local row.
func TestDBInboundRemoteCommentDelete(t *testing.T) {
	ctx := context.Background()
	svc, _, _, uri := a5Fixture(t, ctx)
	svc.SetNotifier(&captureNotifier{})

	remoteDID := fmt.Sprintf("did:plc:dl%d", time.Now().UnixNano())
	replyURI := "at://" + remoteDID + "/app.bsky.feed.post/dr1"
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "dr1", 600, colFeedPost, map[string]any{
		"text":  "will delete",
		"reply": map[string]any{"parent": map[string]any{"uri": uri}},
	}))

	del := commitEventWithURI(remoteDID, "dr1", 601, colFeedPost, nil)
	del.Commit.Operation = "delete"
	svc.handleCommit(ctx, del)

	d := testDB(t)
	var deleted *time.Time
	_ = d.Writer.QueryRow(ctx, `SELECT deleted_at FROM comments WHERE remote_uri = $1`, replyURI).Scan(&deleted)
	if deleted == nil {
		t.Fatalf("remote comment not tombstoned")
	}
}

// commitEventWithURI builds a commit event for collection/rkey.
func commitEventWithURI(did, rkey string, seq int64, collection string, record map[string]any) *jetstreamEvent {
	ev := commitEvent(did, rkey, seq, record)
	ev.Commit.Collection = collection
	return ev
}

// TestDBInboundNestedReplyCounters — a reply to a remote comment nests at
// depth+1 and bumps parent reply_count + content comment_count; the delete
// unwinds them.
func TestDBInboundNestedReplyCounters(t *testing.T) {
	ctx := context.Background()
	svc, _, contentID, uri := a5Fixture(t, ctx)
	svc.SetNotifier(&captureNotifier{})

	remoteDID := fmt.Sprintf("did:plc:nest%d", time.Now().UnixNano())
	parentURI := "at://" + remoteDID + "/app.bsky.feed.post/p1"
	// Remote reply directly on the member post → depth 0 remote comment.
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "p1", 700, colFeedPost, map[string]any{
		"text":  "parent reply",
		"reply": map[string]any{"parent": map[string]any{"uri": uri}, "root": map[string]any{"uri": uri}},
	}))
	// Nested reply whose parent is the remote comment → depth 1.
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "p2", 701, colFeedPost, map[string]any{
		"text":  "nested reply",
		"reply": map[string]any{"parent": map[string]any{"uri": parentURI}, "root": map[string]any{"uri": uri}},
	}))

	d := testDB(t)
	var depth int
	var parentID pgtype.UUID
	err := d.Writer.QueryRow(ctx, `SELECT depth, parent_id FROM comments WHERE remote_uri = $1`,
		"at://"+remoteDID+"/app.bsky.feed.post/p2").Scan(&depth, &parentID)
	if err != nil {
		t.Fatalf("nested comment missing: %v", err)
	}
	if depth != 1 || !parentID.Valid {
		t.Fatalf("nested reply depth=%d parent=%v", depth, parentID)
	}
	var rc int
	_ = d.Writer.QueryRow(ctx, `SELECT reply_count FROM comments WHERE remote_uri = $1`, parentURI).Scan(&rc)
	if rc != 1 {
		t.Fatalf("parent reply_count=%d", rc)
	}
	var cc int
	_ = d.Writer.QueryRow(ctx, `SELECT comment_count FROM contents WHERE id = $1`, contentID).Scan(&cc)
	if cc != 2 {
		t.Fatalf("content comment_count=%d", cc)
	}

	// Deleting the nested reply unwinds both counters.
	del := commitEventWithURI(remoteDID, "p2", 702, colFeedPost, nil)
	del.Commit.Operation = "delete"
	svc.handleCommit(ctx, del)
	_ = d.Writer.QueryRow(ctx, `SELECT reply_count FROM comments WHERE remote_uri = $1`, parentURI).Scan(&rc)
	_ = d.Writer.QueryRow(ctx, `SELECT comment_count FROM contents WHERE id = $1`, contentID).Scan(&cc)
	if rc != 0 || cc != 1 {
		t.Fatalf("counters not unwound: reply_count=%d comment_count=%d", rc, cc)
	}
}

// TestDBInboundReplayNewSeqNoDupNotify — a replayed create carrying a fresh
// upstream time_us with the same record URI must not re-notify.
func TestDBInboundReplayNewSeqNoDupNotify(t *testing.T) {
	ctx := context.Background()
	svc, _, _, uri := a5Fixture(t, ctx)
	notifier := &captureNotifier{}
	svc.SetNotifier(notifier)

	remoteDID := fmt.Sprintf("did:plc:rp%d", time.Now().UnixNano())
	like := map[string]any{"subject": map[string]any{"uri": uri}}
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "lk1", 800, colFeedLike, like))
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "lk1", 801, colFeedLike, like)) // replay, new seq

	if len(notifier.calls) != 1 {
		t.Fatalf("replayed like notified %d times", len(notifier.calls))
	}
	d := testDB(t)
	var status string
	_ = d.Writer.QueryRow(ctx, `SELECT status FROM atproto_inbound WHERE event_seq = 801 AND did = $1`, remoteDID).Scan(&status)
	if status != "ignored" {
		t.Fatalf("replay ledger status=%s", status)
	}
}

// TestDBInboundUpdateOps — an update op refreshes the comment body with
// is_edited; an update for an unseen URI falls through to the create path.
func TestDBInboundUpdateOps(t *testing.T) {
	ctx := context.Background()
	svc, _, _, uri := a5Fixture(t, ctx)
	svc.SetNotifier(&captureNotifier{})

	remoteDID := fmt.Sprintf("did:plc:upd%d", time.Now().UnixNano())
	replyURI := "at://" + remoteDID + "/app.bsky.feed.post/u1"
	svc.handleCommit(ctx, commitEventWithURI(remoteDID, "u1", 900, colFeedPost, map[string]any{
		"text":  "original text",
		"reply": map[string]any{"parent": map[string]any{"uri": uri}},
	}))

	d := testDB(t)
	// Edit lands → body refreshes + is_edited set.
	upd := commitEventWithURI(remoteDID, "u1", 901, colFeedPost, map[string]any{
		"text":  "edited text",
		"reply": map[string]any{"parent": map[string]any{"uri": uri}},
	})
	upd.Commit.Operation = "update"
	svc.handleCommit(ctx, upd)

	var body []byte
	var edited bool
	_ = d.Writer.QueryRow(ctx, `SELECT body, is_edited FROM comments WHERE remote_uri = $1`, replyURI).Scan(&body, &edited)
	if !strings.Contains(string(body), "edited text") || !edited {
		t.Fatalf("update not applied: body=%q edited=%v", body, edited)
	}

	// Update for a never-seen reply → creates the comment (catch-up).
	upd2 := commitEventWithURI(remoteDID, "u2", 902, colFeedPost, map[string]any{
		"text":  "missed create",
		"reply": map[string]any{"parent": map[string]any{"uri": uri}},
	})
	upd2.Commit.Operation = "update"
	svc.handleCommit(ctx, upd2)
	var exists bool
	_ = d.Writer.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM comments WHERE remote_uri = $1)`,
		"at://"+remoteDID+"/app.bsky.feed.post/u2").Scan(&exists)
	if !exists {
		t.Fatalf("update-fallback create missing")
	}

	// Update on a deleted comment must not resurrect it.
	del := commitEventWithURI(remoteDID, "u1", 903, colFeedPost, nil)
	del.Commit.Operation = "delete"
	svc.handleCommit(ctx, del)
	upd3 := commitEventWithURI(remoteDID, "u1", 904, colFeedPost, map[string]any{
		"text":  "zombie",
		"reply": map[string]any{"parent": map[string]any{"uri": uri}},
	})
	upd3.Commit.Operation = "update"
	svc.handleCommit(ctx, upd3)
	var deleted *time.Time
	_ = d.Writer.QueryRow(ctx, `SELECT deleted_at FROM comments WHERE remote_uri = $1`, replyURI).Scan(&deleted)
	if deleted == nil {
		t.Fatalf("update resurrected a deleted comment")
	}
}
