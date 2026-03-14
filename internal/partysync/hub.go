package partysync

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/kirill/gamelogserver/internal/partysync/pb"
)

// DefaultBroadcastInterval is the max frequency at which PartyState is sent to clients.
const DefaultBroadcastInterval = 300 * time.Millisecond

const (
	maxRooms          = 1000
	maxPlayersPerRoom = 15
	snapshotTTL       = 60 * time.Second
	roomTTL           = 5 * time.Minute
)

// Room holds all active streams and latest snapshots for one party room.
type Room struct {
	streams      map[string]pb.PartySync_SyncDamageServer
	snapshots    map[string]*pb.PlayerSnapshot
	lastUpdated  map[string]time.Time
	lastActivity time.Time
	done         chan struct{}
	mu           sync.RWMutex
	dirty        atomic.Bool
	cancel       context.CancelFunc
}

// Hub manages all rooms.
type Hub struct {
	rooms    map[string]*Room
	mu       sync.RWMutex
	interval time.Duration
}

func NewHub() *Hub {
	return newHubWithInterval(DefaultBroadcastInterval)
}

func newHubWithInterval(interval time.Duration) *Hub {
	return &Hub{
		rooms:    make(map[string]*Room),
		interval: interval,
	}
}

func (h *Hub) getOrCreateRoom(roomID string) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.rooms[roomID]; ok {
		return r
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Room{
		streams:      make(map[string]pb.PartySync_SyncDamageServer),
		snapshots:    make(map[string]*pb.PlayerSnapshot),
		lastUpdated:  make(map[string]time.Time),
		lastActivity: time.Now(),
		done:         make(chan struct{}),
		cancel:       cancel,
	}
	h.rooms[roomID] = r
	go r.broadcastLoop(ctx, h.interval, func() { h.closeRoom(roomID, r) })
	return r
}

// closeRoom cancels the room and removes it from the hub.
func (h *Hub) closeRoom(roomID string, r *Room) {
	r.cancel()
	close(r.done)
	h.mu.Lock()
	delete(h.rooms, roomID)
	h.mu.Unlock()
}

// broadcastLoop ticks every interval, evicts stale snapshots, closes inactive rooms, and broadcasts if dirty.
func (r *Room) broadcastLoop(ctx context.Context, interval time.Duration, closeRoom func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.RLock()
			inactive := time.Since(r.lastActivity) > roomTTL
			r.mu.RUnlock()
			if inactive {
				closeRoom()
				return
			}
			if r.evictStale() {
				r.dirty.Store(true)
			}
			if r.dirty.Swap(false) {
				r.broadcast()
			}
		}
	}
}

// evictStale removes snapshots not updated within snapshotTTL. Returns true if any were removed.
func (r *Room) evictStale() bool {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	evicted := false
	for player, t := range r.lastUpdated {
		if now.Sub(t) > snapshotTTL {
			delete(r.snapshots, player)
			delete(r.lastUpdated, player)
			evicted = true
		}
	}
	return evicted
}

// Join registers a player stream in the room.
// Returns a done channel closed when the room is force-closed, or an error if the room is full.
func (h *Hub) Join(roomID, player string, stream pb.PartySync_SyncDamageServer) (<-chan struct{}, error) {
	h.mu.RLock()
	roomCount := len(h.rooms)
	_, exists := h.rooms[roomID]
	h.mu.RUnlock()

	if !exists && roomCount >= maxRooms {
		return nil, fmt.Errorf("server room limit reached")
	}

	r := h.getOrCreateRoom(roomID)
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, taken := r.streams[player]; taken {
		return nil, fmt.Errorf("player name already taken in this room")
	}
	if len(r.streams) >= maxPlayersPerRoom {
		return nil, fmt.Errorf("room is full (max %d players)", maxPlayersPerRoom)
	}
	r.streams[player] = stream
	return r.done, nil
}

// Leave removes a player from the room. Deletes the room when it becomes empty.
func (h *Hub) Leave(roomID, player string) {
	h.mu.RLock()
	r, ok := h.rooms[roomID]
	h.mu.RUnlock()
	if !ok {
		return
	}

	r.mu.Lock()
	delete(r.streams, player)
	delete(r.snapshots, player)
	delete(r.lastUpdated, player)
	empty := len(r.streams) == 0
	r.mu.Unlock()

	if empty {
		h.closeRoom(roomID, r)
	}
}

// Update saves a player snapshot and marks the room dirty for the next broadcast tick.
func (h *Hub) Update(roomID string, msg *pb.DamageUpdate) {
	r := h.getOrCreateRoom(roomID)

	snap := &pb.PlayerSnapshot{
		TotalDamage: msg.TotalDamage,
		Targets:     msg.Targets,
		LastHit:     msg.LastHit,
		LastHitTs:   msg.LastHitTs,
	}

	r.mu.Lock()
	r.snapshots[msg.Player] = snap
	r.lastUpdated[msg.Player] = time.Now()
	r.lastActivity = time.Now()
	r.mu.Unlock()

	r.dirty.Store(true)
}

// broadcast sends the current PartyState to all streams in the room.
func (r *Room) broadcast() {
	r.mu.RLock()
	state := &pb.PartyState{
		Players: make(map[string]*pb.PlayerSnapshot, len(r.snapshots)),
	}
	for name, snap := range r.snapshots {
		state.Players[name] = snap
	}
	streams := make(map[string]pb.PartySync_SyncDamageServer, len(r.streams))
	for name, s := range r.streams {
		streams[name] = s
	}
	r.mu.RUnlock()

	for _, stream := range streams {
		_ = stream.Send(state)
	}
}
