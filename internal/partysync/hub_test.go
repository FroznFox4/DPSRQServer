package partysync

import (
	"context"
	"sync"
	"testing"

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

func TestHub_JoinAndUpdate_BroadcastsToJoinedStream(t *testing.T) {
	hub := NewHub()
	s := newMockStream()

	hub.Join("room1", "Alice", s)
	hub.Update("room1", &pb.DamageUpdate{
		Player:      "Alice",
		Room:        "room1",
		TotalDamage: 500,
	})

	if s.sentCount() != 1 {
		t.Fatalf("expected 1 broadcast, got %d", s.sentCount())
	}
	state := s.lastSent()
	snap, ok := state.Players["Alice"]
	if !ok {
		t.Fatal("Alice not found in PartyState")
	}
	if snap.TotalDamage != 500 {
		t.Errorf("expected TotalDamage=500, got %d", snap.TotalDamage)
	}
}

func TestHub_MultiplePlayersReceiveAllBroadcasts(t *testing.T) {
	hub := NewHub()
	sA := newMockStream()
	sB := newMockStream()

	hub.Join("room1", "Alice", sA)
	hub.Join("room1", "Bob", sB)

	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 100})
	hub.Update("room1", &pb.DamageUpdate{Player: "Bob", Room: "room1", TotalDamage: 200})

	if sA.sentCount() != 2 {
		t.Errorf("Alice: expected 2 broadcasts, got %d", sA.sentCount())
	}
	if sB.sentCount() != 2 {
		t.Errorf("Bob: expected 2 broadcasts, got %d", sB.sentCount())
	}

	// Last state must contain both players
	last := sA.lastSent()
	if _, ok := last.Players["Alice"]; !ok {
		t.Error("Alice missing from final state")
	}
	if _, ok := last.Players["Bob"]; !ok {
		t.Error("Bob missing from final state")
	}
}

func TestHub_PartyStateContainsLatestSnapshot(t *testing.T) {
	hub := NewHub()
	s := newMockStream()
	hub.Join("room1", "Alice", s)

	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 100})
	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 900})

	last := s.lastSent()
	if last.Players["Alice"].TotalDamage != 900 {
		t.Errorf("expected latest snapshot TotalDamage=900, got %d", last.Players["Alice"].TotalDamage)
	}
}

func TestHub_Leave_StopsReceivingBroadcasts(t *testing.T) {
	hub := NewHub()
	s := newMockStream()

	hub.Join("room1", "Alice", s)
	hub.Leave("room1", "Alice")
	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 999})

	if s.sentCount() != 0 {
		t.Errorf("expected 0 broadcasts after Leave, got %d", s.sentCount())
	}
}

func TestHub_Leave_RemovesSnapshotFromBroadcast(t *testing.T) {
	hub := NewHub()
	sA := newMockStream()
	sB := newMockStream()

	hub.Join("room1", "Alice", sA)
	hub.Join("room1", "Bob", sB)
	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 100})
	hub.Update("room1", &pb.DamageUpdate{Player: "Bob", Room: "room1", TotalDamage: 200})

	hub.Leave("room1", "Alice")
	// Bob sends update — state should only contain Bob
	hub.Update("room1", &pb.DamageUpdate{Player: "Bob", Room: "room1", TotalDamage: 300})

	last := sB.lastSent()
	if _, ok := last.Players["Alice"]; ok {
		t.Error("Alice should not appear in state after leaving")
	}
	if snap, ok := last.Players["Bob"]; !ok || snap.TotalDamage != 300 {
		t.Errorf("Bob should have TotalDamage=300 after Alice left, got %+v", snap)
	}
}

func TestHub_EmptyRoomDeletedAfterLastLeave(t *testing.T) {
	hub := NewHub()
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
	hub := NewHub()
	// Should not panic even with no streams in the room
	hub.Update("nonexistent", &pb.DamageUpdate{Player: "Ghost", Room: "nonexistent", TotalDamage: 1})
}

func TestHub_LeaveUnknownRoom_DoesNotPanic(t *testing.T) {
	hub := NewHub()
	hub.Leave("nonexistent", "Ghost")
}

func TestHub_TargetDamagePreservedInSnapshot(t *testing.T) {
	hub := NewHub()
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
	hub := NewHub()
	s1 := newMockStream()
	s2 := newMockStream()

	hub.Join("room1", "Alice", s1)
	hub.Join("room2", "Bob", s2)

	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 100})

	if s1.sentCount() != 1 {
		t.Errorf("room1: expected 1 broadcast, got %d", s1.sentCount())
	}
	if s2.sentCount() != 0 {
		t.Errorf("room2: expected 0 broadcasts (isolated), got %d", s2.sentCount())
	}
}
