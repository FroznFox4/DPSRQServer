package partysync

import (
	"fmt"
	"io"

	pb "github.com/kirill/gamelogserver/internal/partysync/pb"
)

const (
	maxPlayerLen = 50
	maxRoomLen   = 100
	maxTargets   = 50
)

type partySyncService struct {
	pb.UnimplementedPartySyncServer
	hub *Hub
}

func NewService(hub *Hub) pb.PartySyncServer {
	return &partySyncService{hub: hub}
}

func (s *partySyncService) SyncDamage(stream pb.PartySync_SyncDamageServer) error {
	firstMsg, err := stream.Recv()
	if err != nil {
		return err
	}
	room := firstMsg.Room
	if room == "" {
		return fmt.Errorf("room must not be empty")
	}
	if len(room) > maxRoomLen {
		return fmt.Errorf("room name too long (max %d)", maxRoomLen)
	}
	username := firstMsg.Player
	if username == "" {
		return fmt.Errorf("player must not be empty")
	}
	if len(username) > maxPlayerLen {
		return fmt.Errorf("player name too long (max %d)", maxPlayerLen)
	}

	done, err := s.hub.Join(room, username, stream)
	if err != nil {
		return err
	}
	defer s.hub.Leave(room, username)

	s.hub.Update(room, firstMsg)

	type recvResult struct {
		msg *pb.DamageUpdate
		err error
	}
	recvCh := make(chan recvResult, 1)
	recv := func() {
		msg, err := stream.Recv()
		recvCh <- recvResult{msg, err}
	}
	go recv()

	for {
		select {
		case <-done:
			return fmt.Errorf("room closed due to inactivity")
		case r := <-recvCh:
			if r.err == io.EOF {
				return nil
			}
			if r.err != nil {
				return r.err
			}
			r.msg.Player = username
			if len(r.msg.Targets) > maxTargets {
				r.msg.Targets = r.msg.Targets[:maxTargets]
			}
			s.hub.Update(room, r.msg)
			go recv()
		}
	}
}
