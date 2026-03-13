package partysync

import (
	"fmt"
	"io"
	"strings"

	"google.golang.org/grpc/metadata"

	pb "github.com/kirill/gamelogserver/internal/partysync/pb"
	"github.com/kirill/gamelogserver/internal/service"
)

type partySyncService struct {
	pb.UnimplementedPartySyncServer
	hub         *Hub
	authService *service.AuthService
}

func NewService(hub *Hub, authService *service.AuthService) pb.PartySyncServer {
	return &partySyncService{hub: hub, authService: authService}
}

func (s *partySyncService) SyncDamage(stream pb.PartySync_SyncDamageServer) error {
	// 1. Extract JWT from metadata
	md, ok := metadata.FromIncomingContext(stream.Context())
	if !ok {
		return fmt.Errorf("missing metadata")
	}
	tokens := md.Get("authorization")
	if len(tokens) == 0 {
		return fmt.Errorf("missing authorization")
	}
	tokenStr := strings.TrimPrefix(tokens[0], "Bearer ")
	_, username, err := s.authService.ValidateToken(tokenStr)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	// 2. First message to get room
	firstMsg, err := stream.Recv()
	if err != nil {
		return err
	}
	room := firstMsg.Room
	if room == "" {
		return fmt.Errorf("room must not be empty")
	}

	// 3. Join room
	s.hub.Join(room, username, stream)
	defer s.hub.Leave(room, username)

	// Process first message
	firstMsg.Player = username
	s.hub.Update(room, firstMsg)

	// 4. Main recv loop
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
