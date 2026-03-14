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

func mustJoin(t *testing.T, hub *Hub, room, player string, s pb.PartySync_SyncDamageServer) <-chan struct{} {
	t.Helper()
	done, err := hub.Join(room, player, s)
	if err != nil {
		t.Fatalf("Join(%q, %q) unexpected error: %v", room, player, err)
	}
	return done
}

// --- existing tests ---

func TestHub_JoinAndUpdate_BroadcastsToJoinedStream(t *testing.T) {
	hub := newTestHub()
	s := newMockStream()

	mustJoin(t, hub, "room1", "Alice", s)
	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 500})
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

	mustJoin(t, hub, "room1", "Alice", sA)
	mustJoin(t, hub, "room1", "Bob", sB)

	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 100})
	hub.Update("room1", &pb.DamageUpdate{Player: "Bob", Room: "room1", TotalDamage: 200})
	tick()

	if sA.sentCount() == 0 {
		t.Error("Alice: expected at least 1 broadcast, got 0")
	}
	if sB.sentCount() == 0 {
		t.Error("Bob: expected at least 1 broadcast, got 0")
	}

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
	mustJoin(t, hub, "room1", "Alice", s)

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
	mustJoin(t, hub, "room1", "Alice", s)

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

	mustJoin(t, hub, "room1", "Alice", s)
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

	mustJoin(t, hub, "room1", "Alice", sA)
	mustJoin(t, hub, "room1", "Bob", sB)
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

	mustJoin(t, hub, "room1", "Alice", s)
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
	mustJoin(t, hub, "room1", "Alice", s)

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

	mustJoin(t, hub, "room1", "Alice", s1)
	mustJoin(t, hub, "room2", "Bob", s2)

	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 100})
	tick()

	if s1.sentCount() == 0 {
		t.Errorf("room1: expected at least 1 broadcast, got 0")
	}
	if s2.sentCount() != 0 {
		t.Errorf("room2: expected 0 broadcasts (isolated), got %d", s2.sentCount())
	}
}

// --- new tests ---

func TestHub_DuplicatePlayerName_ReturnsError(t *testing.T) {
	hub := newTestHub()
	s1 := newMockStream()
	s2 := newMockStream()

	mustJoin(t, hub, "room1", "Alice", s1)

	_, err := hub.Join("room1", "Alice", s2)
	if err == nil {
		t.Fatal("expected error when joining with duplicate player name, got nil")
	}
}

func TestHub_MaxPlayersPerRoom_ReturnsError(t *testing.T) {
	hub := newTestHub()

	for i := 0; i < maxPlayersPerRoom; i++ {
		s := newMockStream()
		name := string(rune('A' + i))
		if _, err := hub.Join("room1", name, s); err != nil {
			t.Fatalf("Join player %d failed unexpectedly: %v", i, err)
		}
	}

	extra := newMockStream()
	_, err := hub.Join("room1", "Extra", extra)
	if err == nil {
		t.Fatal("expected error when exceeding max players, got nil")
	}
}

func TestHub_RoomClosedAfterInactivity(t *testing.T) {
	shortRoomTTL := 30 * time.Millisecond
	hub := newHubWithAll(5*time.Millisecond, snapshotTTL, shortRoomTTL)
	s := newMockStream()

	done, err := hub.Join("room1", "Alice", s)
	if err != nil {
		t.Fatalf("Join failed: %v", err)
	}

	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 100})

	// Wait longer than roomTTL without sending updates
	time.Sleep(60 * time.Millisecond)

	select {
	case <-done:
		// expected: room was force-closed
	default:
		t.Fatal("expected done channel to be closed after inactivity, but it is still open")
	}

	hub.mu.RLock()
	_, exists := hub.rooms["room1"]
	hub.mu.RUnlock()
	if exists {
		t.Error("expected room to be removed from hub after inactivity close")
	}
}

func TestHub_RoomNotClosedWhileActive(t *testing.T) {
	shortRoomTTL := 30 * time.Millisecond
	hub := newHubWithAll(5*time.Millisecond, snapshotTTL, shortRoomTTL)
	s := newMockStream()

	done, err := hub.Join("room1", "Alice", s)
	if err != nil {
		t.Fatalf("Join failed: %v", err)
	}

	// Send updates continuously to keep the room active
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 100})
			}
		}
	}()

	time.Sleep(60 * time.Millisecond)
	close(stop)

	select {
	case <-done:
		t.Fatal("room was force-closed despite ongoing activity")
	default:
		// expected: room is still open
	}
}

func TestHub_SnapshotEvictedAfterTTL(t *testing.T) {
	shortSnapshotTTL := 30 * time.Millisecond
	hub := newHubWithAll(5*time.Millisecond, shortSnapshotTTL, roomTTL)
	sA := newMockStream()
	sB := newMockStream()

	mustJoin(t, hub, "room1", "Alice", sA)
	mustJoin(t, hub, "room1", "Bob", sB)

	hub.Update("room1", &pb.DamageUpdate{Player: "Alice", Room: "room1", TotalDamage: 100})
	hub.Update("room1", &pb.DamageUpdate{Player: "Bob", Room: "room1", TotalDamage: 200})
	tick()

	// Only Bob keeps sending updates; Alice goes silent
	time.Sleep(60 * time.Millisecond)
	hub.Update("room1", &pb.DamageUpdate{Player: "Bob", Room: "room1", TotalDamage: 300})
	tick()

	last := sB.lastSent()
	if _, ok := last.Players["Alice"]; ok {
		t.Error("Alice snapshot should have been evicted after TTL")
	}
	if snap, ok := last.Players["Bob"]; !ok || snap.TotalDamage != 300 {
		t.Errorf("Bob should still be present with TotalDamage=300, got %+v", snap)
	}
}
