package partysync

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/kirill/gamelogserver/internal/partysync/pb"
)

// DefaultBroadcastInterval is the max frequency at which PartyState is sent to clients.
const DefaultBroadcastInterval = 300 * time.Millisecond

// Room holds all active streams and latest snapshots for one party room.
type Room struct {
	streams   map[string]pb.PartySync_SyncDamageServer
	snapshots map[string]*pb.PlayerSnapshot
	mu        sync.RWMutex
	dirty     atomic.Bool
	cancel    context.CancelFunc
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
		streams:   make(map[string]pb.PartySync_SyncDamageServer),
		snapshots: make(map[string]*pb.PlayerSnapshot),
		cancel:    cancel,
	}
	h.rooms[roomID] = r
	go r.broadcastLoop(ctx, h.interval)
	return r
}

// broadcastLoop ticks every interval and sends PartyState if the room was updated.
func (r *Room) broadcastLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.dirty.Swap(false) {
				r.broadcast()
			}
		}
	}
}

// Join registers a player stream in the room.
func (h *Hub) Join(roomID, player string, stream pb.PartySync_SyncDamageServer) {
	r := h.getOrCreateRoom(roomID)
	r.mu.Lock()
	r.streams[player] = stream
	r.mu.Unlock()
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
	empty := len(r.streams) == 0
	r.mu.Unlock()

	if empty {
		r.cancel()
		h.mu.Lock()
		delete(h.rooms, roomID)
		h.mu.Unlock()
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
