package websocket

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/valkey-io/valkey-go/valkeycompat"
)

// PubSubChannel is the Valkey channel used for cross-instance WebSocket
// broadcast delivery. All instances subscribe to this single channel and
// dispatch received messages to their local hub.
const PubSubChannel = "jc:ws:broadcast"

// PubSubMessage wraps a WSEvent with routing metadata for cross-instance
// delivery. When an event is published to Valkey, the bridge attaches the
// target user or space ID so that receiving instances can dispatch it to
// the correct local subscribers.
type PubSubMessage struct {
	Type          string      `json:"type"`
	TargetUserID  string      `json:"target_user_id,omitempty"`
	TargetSpaceID string      `json:"target_space_id,omitempty"`
	Payload       interface{} `json:"payload"`
}

// PubSubBridge connects the local WebSocket hub to Valkey pub/sub for
// multi-instance scaling. Each instance publishes its outbound events to
// Valkey and subscribes to the same channel, dispatching received messages
// to local hub clients.
type PubSubBridge struct {
	rdb valkeycompat.Cmdable
	hub *Hub
	sub valkeycompat.PubSub
}

// NewPubSubBridge creates a bridge between the given Valkey client and
// WebSocket hub. Call Start to begin subscribing.
func NewPubSubBridge(rdb valkeycompat.Cmdable, hub *Hub) *PubSubBridge {
	return &PubSubBridge{
		rdb: rdb,
		hub: hub,
	}
}

// Start subscribes to the broadcast channel and begins dispatching received
// messages to the local hub. The goroutine exits when ctx is cancelled.
// Safe to call from a goroutine.
func (b *PubSubBridge) Start(ctx context.Context) {
	b.sub = b.rdb.Subscribe(ctx, PubSubChannel)

	ch := b.sub.Channel()

	slog.Info("websocket pubsub bridge started", "channel", PubSubChannel)

	go func() {
		defer func() { _ = b.sub.Close() }()

		for {
			select {
			case <-ctx.Done():
				slog.Info("websocket pubsub bridge stopped")
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}

				var incoming PubSubMessage
				if err := json.Unmarshal([]byte(msg.Payload), &incoming); err != nil {
					slog.Error("websocket pubsub unmarshal", "error", err)
					continue
				}

				b.dispatch(incoming)
			}
		}
	}()
}

// Publish serialises the event as a PubSubMessage and publishes it to
// Valkey for delivery to other instances. The local hub is NOT called
// here — callers invoke BroadcastToUser/BroadcastToSpace/BroadcastToAll
// for local delivery and call Publish separately for cross-instance.
func (b *PubSubBridge) Publish(event WSEvent, targetUserID, targetSpaceID string) {
	msg := PubSubMessage{
		Type:          event.Type,
		TargetUserID:  targetUserID,
		TargetSpaceID: targetSpaceID,
		Payload:       event.Payload,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("websocket pubsub marshal", "error", err)
		return
	}

	if err := b.rdb.Publish(context.Background(), PubSubChannel, data).Err(); err != nil {
		slog.Error("websocket pubsub publish", "error", err)
	}
}

// dispatch delivers a received PubSubMessage to the local hub. This is the
// cross-instance counterpart of BroadcastToUser/BroadcastToSpace/BroadcastToAll:
// same logic, but invoked from the subscription goroutine rather than the
// publishing instance.
func (b *PubSubBridge) dispatch(msg PubSubMessage) {
	event := WSEvent{Type: msg.Type, Payload: msg.Payload}

	switch {
	case msg.TargetUserID != "":
		b.hub.BroadcastToUser(msg.TargetUserID, event)
	case msg.TargetSpaceID != "":
		b.hub.BroadcastToSpace(msg.TargetSpaceID, event)
	default:
		b.hub.BroadcastToAll(event)
	}
}
