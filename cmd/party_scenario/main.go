// party_scenario — integration test: 3 players join a room, send damage, verify PartyState.
// Run with: go run ./cmd/party_scenario
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/kirill/gamelogserver/internal/partysync/pb"
)

const (
	grpcAddr = "localhost:50051"
	room     = "test-room"
)

type player struct {
	name        string
	totalDamage int64
	targets     []*pb.TargetDamage
}

func main() {
	players := []player{
		{name: "Alice", totalDamage: 85000, targets: []*pb.TargetDamage{
			{Name: "Dragon", Amount: 60000},
			{Name: "Orc", Amount: 25000},
		}},
		{name: "Bob", totalDamage: 42000, targets: []*pb.TargetDamage{
			{Name: "Dragon", Amount: 42000},
		}},
		{name: "Carol", totalDamage: 28000, targets: []*pb.TargetDamage{
			{Name: "Orc", Amount: 28000},
		}},
	}

	conn, err := grpc.NewClient(grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("grpc dial: %v", err)
	}
	defer conn.Close()

	fmt.Println("━━━ [1] Opening gRPC streams ━━━")
	type result struct {
		playerName string
		state      *pb.PartyState
		err        error
	}
	results := make(chan result, len(players))

	var wg sync.WaitGroup
	for _, p := range players {
		wg.Add(1)
		go func(p player) {
			defer wg.Done()
			state, err := runPlayer(conn, p)
			results <- result{playerName: p.name, state: state, err: err}
		}(p)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	fmt.Println("\n━━━ [2] Party state received by each player ━━━")
	allOk := true
	for r := range results {
		if r.err != nil {
			fmt.Printf("  ✗ %s: ERROR — %v\n", r.playerName, r.err)
			allOk = false
			continue
		}
		fmt.Printf("\n  Player %s sees:\n", r.playerName)
		for name, snap := range r.state.Players {
			fmt.Printf("    %-8s  total=%d  last_hit=%d  targets=%v\n",
				name, snap.TotalDamage, snap.LastHit, formatTargets(snap.Targets))
		}

		if len(r.state.Players) != 3 {
			fmt.Printf("  ✗ %s: expected 3 players in PartyState, got %d\n", r.playerName, len(r.state.Players))
			allOk = false
		} else {
			fmt.Printf("  ✓ %s: sees all 3 players\n", r.playerName)
		}
	}

	fmt.Println()
	if allOk {
		fmt.Println("━━━ RESULT: ALL CHECKS PASSED ✓ ━━━")
	} else {
		fmt.Println("━━━ RESULT: SOME CHECKS FAILED ✗ ━━━")
	}
}

func runPlayer(conn *grpc.ClientConn, p player) (*pb.PartyState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := pb.NewPartySyncClient(conn)
	stream, err := client.SyncDamage(ctx)
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}

	err = stream.Send(&pb.DamageUpdate{
		Player:      p.name,
		Room:        room,
		TotalDamage: p.totalDamage,
		Targets:     p.targets,
		LastHit:     p.targets[0].Amount,
		LastHitTs:   time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}

	for {
		state, err := stream.Recv()
		if err == io.EOF {
			return nil, fmt.Errorf("stream closed before full party state")
		}
		if err != nil {
			return nil, fmt.Errorf("recv: %w", err)
		}
		if len(state.Players) == 3 {
			stream.CloseSend()
			return state, nil
		}
		time.Sleep(200 * time.Millisecond)
		_ = stream.Send(&pb.DamageUpdate{
			Player:      p.name,
			Room:        room,
			TotalDamage: p.totalDamage,
			Targets:     p.targets,
			LastHit:     p.targets[0].Amount,
			LastHitTs:   time.Now().Format(time.RFC3339),
		})
	}
}

func formatTargets(targets []*pb.TargetDamage) string {
	var buf bytes.Buffer
	buf.WriteString("[")
	for i, t := range targets {
		if i > 0 {
			buf.WriteString(", ")
		}
		fmt.Fprintf(&buf, "%s:%d", t.Name, t.Amount)
	}
	buf.WriteString("]")
	return buf.String()
}
