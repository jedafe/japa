package atproto

// com.atproto.sync.subscribeRepos (§127 A3): the live firehose relays
// connect to. Frames are two-part CBOR (header {op,t} + body) over one
// binary WS message, sequenced by atproto_repo_events' global identity
// seq — a client supplies ?cursor=<last seen seq> and replays strictly
// newer rows, then live-tails. Commit frames can't skip: their rows
// insert inside the repo-write tx.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md (Sync + discovery)

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

// subscribePollInterval is the live-tail poll — relays buffer frames,
// so sub-second freshness is ample and keeps the DB read cheap.
const subscribePollInterval = 300 * time.Millisecond

// subscribePingInterval keepalives the connection through load
// balancers; relays send nothing.
const subscribePingInterval = 25 * time.Second

// subscribeWriteDeadline bounds one frame write — a stalled consumer
// loses the stream rather than pinning the handler.
const subscribeWriteDeadline = 15 * time.Second

// subscribeBatch is the replay page size; full pages re-poll
// immediately so backlogs drain without waiting a poll cycle.
const subscribeBatch = 500

var xrpcWSUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 8192,
	// Machine-to-machine surface: relay consumers carry no cookies and
	// no Origin — nothing to hijack. (Differs from the user WS upgrader,
	// which gates on the CORS allowlist.)
	CheckOrigin: func(*http.Request) bool { return true },
}

// SubscribeRepos serves GET /xrpc/com.atproto.sync.subscribeRepos
// [?cursor=N] — replay from cursor (exclusive), then live-tail.
func (h *Handlers) SubscribeRepos(w http.ResponseWriter, r *http.Request) {
	var cursor int64
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			xrpcError(w, http.StatusBadRequest, "InvalidRequest", "bad cursor")
			return
		}
		cursor = n
	}

	conn, err := xrpcWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("atproto: subscribeRepos upgrade", "error", err)
		return
	}
	defer conn.Close()
	ctx := r.Context()

	// A client past the high-water mark has skipped sequence space it
	// can't have seen — FutureCursor is the lexicon's error frame.
	if cursor > 0 {
		maxSeq, err := h.service.MaxEventSeq(ctx)
		if err != nil {
			slog.Error("atproto: subscribeRepos max seq", "error", err)
			writeErrorFrame(conn, "InternalError", "internal error")
			return
		}
		if cursor > maxSeq {
			writeErrorFrame(conn, "FutureCursor", "cursor is ahead of the stream")
			return
		}
	}

	h.tailEvents(ctx, conn, cursor)
}

// writeErrorFrame best-effort emits an {op:-1} error frame before close —
// an abrupt drop leaves the relay without diagnostics; the frame names why.
func writeErrorFrame(conn *websocket.Conn, name, message string) {
	frame, err := errorFrameBytes(name, message)
	if err != nil {
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(subscribeWriteDeadline))
	_ = conn.WriteMessage(websocket.BinaryMessage, frame)
}

// tailEvents replays then live-tails the event log onto the socket.
// A read pump detects the peer's close (upgraded connections don't
// cancel r.Context() on their own).
func (h *Handlers) tailEvents(ctx context.Context, conn *websocket.Conn, cursor int64) {
	done := make(chan struct{})
	conn.SetReadLimit(512)
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	poll := time.NewTicker(subscribePollInterval)
	defer poll.Stop()
	ping := time.NewTicker(subscribePingInterval)
	defer ping.Stop()

	last := cursor
	for {
		rows, err := h.service.EventsAfter(ctx, last, subscribeBatch)
		if err != nil {
			slog.Error("atproto: subscribeRepos read events", "error", err)
			writeErrorFrame(conn, "InternalError", "internal error")
			return
		}
		for _, ev := range rows {
			last = ev.Seq
			frame, err := frameBytes(ev)
			if err != nil {
				slog.Warn("atproto: drop malformed event", "seq", ev.Seq, "kind", ev.Kind, "error", err)
				continue
			}
			_ = conn.SetWriteDeadline(time.Now().Add(subscribeWriteDeadline))
			if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				return
			}
		}
		if len(rows) == subscribeBatch {
			continue // backlog page full — drain immediately
		}
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-poll.C:
		case <-ping.C:
			_ = conn.SetWriteDeadline(time.Now().Add(subscribeWriteDeadline))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
