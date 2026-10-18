package websocket

import (
	"sync"
	"testing"
)

// Regression: deliverLocal* used to panic "send on closed channel" when a
// concurrent Unregister closed client.Send between the client snapshot and
// the send (crashed the API — see hub.go safeSend).
func TestSafeSendOnClosedChannel(t *testing.T) {
	t.Parallel()

	c := &Client{UserID: "user-1", Send: make(chan []byte, 1), SpaceIDs: make(map[string]bool)}
	close(c.Send)
	if safeSend(c, []byte(`{"type":"x"}`)) {
		t.Error("safeSend should return false on a closed channel")
	}
	if safeSend(c, []byte(`{"type":"x"}`)) { // idempotent — must not panic twice
		t.Error("safeSend should return false on a closed channel")
	}
}

func TestHubBroadcastUnregisterRace(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	event := WSEvent{Type: "test", Payload: []byte(`{}`)}

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		c := &Client{Hub: hub, UserID: "user-1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
		hub.Register(c)
		wg.Add(2)
		go func() { defer wg.Done(); hub.BroadcastToAll(event) }()
		go func() { defer wg.Done(); hub.Unregister(c) }()
	}
	wg.Wait()
}
