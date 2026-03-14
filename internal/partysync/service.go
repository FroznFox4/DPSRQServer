package partysync

import (
	"fmt"
	"io"

	pb "github.com/kirill/gamelogserver/internal/partysync/pb"
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
	username := firstMsg.Player
	if username == "" {
		return fmt.Errorf("player must not be empty")
	}

	s.hub.Join(room, username, stream)
	defer s.hub.Leave(room, username)

	s.hub.Update(room, firstMsg)

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		msg.Player = username
		s.hub.Update(room, msg)
	}
}
