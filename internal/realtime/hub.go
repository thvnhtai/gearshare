// Package realtime implements the three real-time transport styles the
// requirement checklist asks for on top of one shared in-process broadcast
// hub: Server-Sent Events (owner dashboard booking feed, sse.go), WebSockets
// (live listing availability, websocket.go), and long-polling
// (polling.go, for clients that can't hold either open).
//
// Hub is intentionally a single-process, in-memory fan-out. It is real and
// correct for one API replica; scaling the monolith horizontally would need
// a Redis pub/sub-backed Hub instead (same interface, different transport)
// — noted here rather than pretended away, per the project's "no fake
// breadth" rule.
package realtime

import "sync"

type subscribers map[chan []byte]struct{}

type Hub struct {
	mu          sync.RWMutex
	ownerSubs   map[int64]subscribers
	listingSubs map[int64]subscribers
}

func NewHub() *Hub {
	return &Hub{
		ownerSubs:   make(map[int64]subscribers),
		listingSubs: make(map[int64]subscribers),
	}
}

const subscriberBufferSize = 8

func (h *Hub) SubscribeOwner(ownerID int64) (ch chan []byte, cancel func()) {
	return h.subscribe(h.ownerSubs, ownerID)
}

func (h *Hub) PublishOwner(ownerID int64, payload []byte) {
	h.publish(h.ownerSubs, ownerID, payload)
}

func (h *Hub) SubscribeListing(listingID int64) (ch chan []byte, cancel func()) {
	return h.subscribe(h.listingSubs, listingID)
}

func (h *Hub) PublishListing(listingID int64, payload []byte) {
	h.publish(h.listingSubs, listingID, payload)
}

func (h *Hub) subscribe(subs map[int64]subscribers, key int64) (chan []byte, func()) {
	ch := make(chan []byte, subscriberBufferSize)

	h.mu.Lock()
	if subs[key] == nil {
		subs[key] = make(subscribers)
	}
	subs[key][ch] = struct{}{}
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		delete(subs[key], ch)
		if len(subs[key]) == 0 {
			delete(subs, key)
		}
		h.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

func (h *Hub) publish(subs map[int64]subscribers, key int64, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range subs[key] {
		select {
		case ch <- payload:
		default:
			// Back-pressure policy: a slow subscriber drops the update
			// rather than blocking the publisher (which runs inline in an
			// HTTP handler) — see requirement 19, "back pressure".
		}
	}
}
