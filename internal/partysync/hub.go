package partysync

import (
	"sync"

	pb "github.com/kirill/gamelogserver/internal/partysync/pb"
)

// Room holds all active streams and latest snapshots for one party room.
type Room struct {
	streams   map[string]pb.PartySync_SyncDamageServer
	snapshots map[string]*pb.PlayerSnapshot
	mu        sync.RWMutex
}

// Hub manages all rooms.
type Hub struct {
	rooms map[string]*Room
	mu    sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{rooms: make(map[string]*Room)}
}

func (h *Hub) getOrCreateRoom(roomID string) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.rooms[roomID]; ok {
		return r
	}
	r := &Room{
		streams:   make(map[string]pb.PartySync_SyncDamageServer),
		snapshots: make(map[string]*pb.PlayerSnapshot),
	}
	h.rooms[roomID] = r
	return r
}

// Join registers a player stream in the room.
func (h *Hub) Join(roomID, player string, stream pb.PartySync_SyncDamageServer) {
	r := h.getOrCreateRoom(roomID)
	r.mu.Lock()
	r.streams[player] = stream
	r.mu.Unlock()
}

// Leave removes a player stream from the room.
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
		h.mu.Lock()
		delete(h.rooms, roomID)
		h.mu.Unlock()
	}
}

// Update saves a player snapshot and broadcasts PartyState to all room members.
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

	h.broadcast(r)
}

// broadcast sends current PartyState to all streams in the room.
func (h *Hub) broadcast(r *Room) {
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
		// Non-blocking: ignore errors from individual streams (client may have disconnected)
		_ = stream.Send(state)
	}
}
