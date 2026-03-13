package partysync

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"

	pb "github.com/kirill/gamelogserver/internal/partysync/pb"
)

// mockStream implements pb.PartySync_SyncDamageServer.
// Records every Send call so tests can inspect broadcasts.
type mockStream struct {
	mu   sync.Mutex
	sent []*pb.PartyState
	ctx  context.Context
}

func newMockStream() *mockStream {
	return &mockStream{ctx: context.Background()}
}

func (m *mockStream) Send(ps *pb.PartyState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, ps)
	return nil
}

func (m *mockStream) sentCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

func (m *mockStream) lastSent() *pb.PartyState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return nil
	}
	return m.sent[len(m.sent)-1]
}

// Satisfy the rest of grpc.ServerStream.
func (m *mockStream) Recv() (*pb.DamageUpdate, error)      { return nil, nil }
func (m *mockStream) SetHeader(metadata.MD) error          { return nil }
func (m *mockStream) SendHeader(metadata.MD) error         { return nil }
func (m *mockStream) SetTrailer(metadata.MD)               {}
func (m *mockStream) Context() context.Context             { return m.ctx }
func (m *mockStream) SendMsg(interface{}) error            { return nil }
func (m *mockStream) RecvMsg(interface{}) error            { return nil }

// newTestHub returns a Hub with a very short broadcast interval for unit tests.
func newTestHub() *Hub { return newHubWithInterval(5 * time.Millisecond) }

// tick waits long enough for at least one broadcast tick to fire.
func tick() { time.Sleep(20 * time.Millisecond) }

func TestHub_JoinAndUpdate_BroadcastsToJoinedStream(t *testing.T) {
	hub := newTestHub()
	s := newMockStream()

	hub.Join("room1", "Alice", s)
	hub.Update("room1", &pb.DamageUpdate{
		Player:      "Alice",
		Room:        "room1",
		TotalDamage: 500,
	})
	tick()

	if s.sentCount() == 0 {
		t.Fatal("expected at least 1 broadcast, got 0")
	}
	snap, ok := s.lastSent().Players["Alice"]
	if !ok {
		t.Fatal("Alice not found in PartyState")
	}
	if snap.TotalDamage != 500 {
		t.Errorf("expected TotalDamage=500, got %d", snap.TotalDamage)
	}
}

func TestHub_MultiplePlayersReceiveAllBroadcasts(t *testing.T) {
	hub := newTestHub()
	sA := newMockStream()
	sB := newMockStream()

	hub.Join("room1", "Alice", sA)
	hub.Join("room1", "Bob", sB)

	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 100})
	hub.Update("room1", &pb.DamageUpdate{Player: "Bob", Room: "room1", TotalDamage: 200})
	tick()

	if sA.sentCount() == 0 {
		t.Error("Alice: expected at least 1 broadcast, got 0")
	}
	if sB.sentCount() == 0 {
		t.Error("Bob: expected at least 1 broadcast, got 0")
	}

	// Both players must appear in the last broadcast
	last := sA.lastSent()
	if _, ok := last.Players["Alice"]; !ok {
		t.Error("Alice missing from final state")
	}
	if _, ok := last.Players["Bob"]; !ok {
		t.Error("Bob missing from final state")
	}
}

func TestHub_PartyStateContainsLatestSnapshot(t *testing.T) {
	hub := newTestHub()
	s := newMockStream()
	hub.Join("room1", "Alice", s)

	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 100})
	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 900})
	tick()

	last := s.lastSent()
	if last == nil {
		t.Fatal("expected at least 1 broadcast")
	}
	if last.Players["Alice"].TotalDamage != 900 {
		t.Errorf("expected latest snapshot TotalDamage=900, got %d", last.Players["Alice"].TotalDamage)
	}
}

func TestHub_ThrottlesBatchedUpdatesIntoOneBroadcast(t *testing.T) {
	hub := newTestHub()
	s := newMockStream()
	hub.Join("room1", "Alice", s)

	// Send 10 updates within a single tick window — expect exactly 1 broadcast.
	for i := 0; i < 10; i++ {
		hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: int64(i * 100)})
	}
	tick()

	if s.sentCount() != 1 {
		t.Errorf("expected 1 broadcast for 10 rapid updates, got %d", s.sentCount())
	}
	if s.lastSent().Players["Alice"].TotalDamage != 900 {
		t.Errorf("expected last value 900, got %d", s.lastSent().Players["Alice"].TotalDamage)
	}
}

func TestHub_Leave_StopsReceivingBroadcasts(t *testing.T) {
	hub := newTestHub()
	s := newMockStream()

	hub.Join("room1", "Alice", s)
	hub.Leave("room1", "Alice")
	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 999})
	tick()

	if s.sentCount() != 0 {
		t.Errorf("expected 0 broadcasts after Leave, got %d", s.sentCount())
	}
}

func TestHub_Leave_RemovesSnapshotFromBroadcast(t *testing.T) {
	hub := newTestHub()
	sA := newMockStream()
	sB := newMockStream()

	hub.Join("room1", "Alice", sA)
	hub.Join("room1", "Bob", sB)
	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 100})
	hub.Update("room1", &pb.DamageUpdate{Player: "Bob", Room: "room1", TotalDamage: 200})
	tick()

	hub.Leave("room1", "Alice")
	hub.Update("room1", &pb.DamageUpdate{Player: "Bob", Room: "room1", TotalDamage: 300})
	tick()

	last := sB.lastSent()
	if _, ok := last.Players["Alice"]; ok {
		t.Error("Alice should not appear in state after leaving")
	}
	if snap, ok := last.Players["Bob"]; !ok || snap.TotalDamage != 300 {
		t.Errorf("Bob should have TotalDamage=300 after Alice left, got %+v", snap)
	}
}

func TestHub_EmptyRoomDeletedAfterLastLeave(t *testing.T) {
	hub := newTestHub()
	s := newMockStream()

	hub.Join("room1", "Alice", s)
	hub.Leave("room1", "Alice")

	hub.mu.RLock()
	_, exists := hub.rooms["room1"]
	hub.mu.RUnlock()

	if exists {
		t.Error("expected room to be deleted after last player left")
	}
}

func TestHub_UpdateUnknownRoom_DoesNotPanic(t *testing.T) {
	hub := newTestHub()
	hub.Update("nonexistent", &pb.DamageUpdate{Player: "Ghost", Room: "nonexistent", TotalDamage: 1})
}

func TestHub_LeaveUnknownRoom_DoesNotPanic(t *testing.T) {
	hub := newTestHub()
	hub.Leave("nonexistent", "Ghost")
}

func TestHub_TargetDamagePreservedInSnapshot(t *testing.T) {
	hub := newTestHub()
	s := newMockStream()
	hub.Join("room1", "Alice", s)

	hub.Update("room1", &pb.DamageUpdate{
		Player:      "Alice",
		Room:        "room1",
		TotalDamage: 700,
		Targets: []*pb.TargetDamage{
			{Name: "Dragon", Amount: 500},
			{Name: "Orc", Amount: 200},
		},
		LastHit:   500,
		LastHitTs: "2026-03-13 12:00:00",
	})
	tick()

	snap := s.lastSent().Players["Alice"]
	if len(snap.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(snap.Targets))
	}
	if snap.LastHit != 500 {
		t.Errorf("expected LastHit=500, got %d", snap.LastHit)
	}
	if snap.LastHitTs != "2026-03-13 12:00:00" {
		t.Errorf("unexpected LastHitTs: %s", snap.LastHitTs)
	}
}

func TestHub_MultipleRoomsAreIsolated(t *testing.T) {
	hub := newTestHub()
	s1 := newMockStream()
	s2 := newMockStream()

	hub.Join("room1", "Alice", s1)
	hub.Join("room2", "Bob", s2)

	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 100})
	tick()

	if s1.sentCount() == 0 {
		t.Errorf("room1: expected at least 1 broadcast, got 0")
	}
	if s2.sentCount() != 0 {
		t.Errorf("room2: expected 0 broadcasts (isolated), got %d", s2.sentCount())
	}
}
