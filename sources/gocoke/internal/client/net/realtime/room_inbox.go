package realtime

import "sync"

// roomInbox holds ordered room events while the corresponding scene loads.
// Changing rooms discards the previous room's events, never the new snapshot.
// It is independent of JS/Ebitengine so delayed scene creation is testable.
type roomInbox struct {
	mu      sync.Mutex
	target  string
	packets [][]byte
}

func (q *roomInbox) begin(room string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.target = room
	q.packets = nil
}

// deferPacket returns true when a packet is queued or belongs to an old room.
func (q *roomInbox) deferPacket(room, active string, packet []byte) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if room != q.target {
		return true
	}
	if active == room && len(q.packets) == 0 {
		return false
	}
	q.packets = append(q.packets, append([]byte(nil), packet...))
	return true
}

func (q *roomInbox) drain(active string) [][]byte {
	q.mu.Lock()
	defer q.mu.Unlock()
	if active == "" || active != q.target {
		return nil
	}
	packets := q.packets
	q.packets = nil
	return packets
}
