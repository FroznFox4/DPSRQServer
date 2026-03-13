// throttle_scenario — sends 10 rapid updates from 1 client, counts how many PartyState responses arrive.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	pb "github.com/kirill/gamelogserver/internal/partysync/pb"
)

func main() {
	token := registerOrLogin("throttletest")

	conn, _ := grpc.NewClient("localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))

	client := pb.NewPartySyncClient(conn)
	stream, _ := client.SyncDamage(ctx)

	// Send 10 updates as fast as possible (~microseconds apart)
	const N = 10
	start := time.Now()
	for i := 0; i < N; i++ {
		stream.Send(&pb.DamageUpdate{
			Player:      "throttletest",
			Room:        "throttle-room",
			TotalDamage: int64((i + 1) * 1000),
		})
	}
	sendDuration := time.Since(start)

	// Collect responses for 700ms (covers ~2 broadcast windows of 300ms each)
	received := 0
	var lastDamage int64
	collectCtx, collectCancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer collectCancel()

	recvCh := make(chan *pb.PartyState, 20)
	go func() {
		for {
			state, err := stream.Recv()
			if err != nil {
				close(recvCh)
				return
			}
			recvCh <- state
		}
	}()

	for {
		select {
		case <-collectCtx.Done():
			goto done
		case state, ok := <-recvCh:
			if !ok {
				goto done
			}
			received++
			if snap := state.Players["throttletest"]; snap != nil {
				lastDamage = snap.TotalDamage
			}
		}
	}
done:
	stream.CloseSend()

	fmt.Printf("Sent %d updates in %v\n", N, sendDuration.Round(time.Microsecond))
	fmt.Printf("Received %d PartyState messages in 700ms (throttle window: 300ms → expect ≤2)\n", received)
	fmt.Printf("Last TotalDamage: %d (expected %d)\n", lastDamage, int64(N*1000))
	fmt.Println()

	ok := received <= 2 && lastDamage == int64(N*1000)
	if ok {
		fmt.Printf("✓ Throttle OK: %d rapid updates batched into %d broadcast(s)\n", N, received)
	} else {
		fmt.Printf("✗ Throttle FAIL: received=%d lastDamage=%d\n", received, lastDamage)
	}
}

func registerOrLogin(username string) string {
	body, _ := json.Marshal(map[string]string{"username": username, "password": "secret123"})
	resp, _ := http.Post("http://localhost:8080/api/v1/auth/register", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	json.Unmarshal(raw, &out)
	if tok, ok := out["token"].(string); ok {
		return tok
	}
	resp2, _ := http.Post("http://localhost:8080/api/v1/auth/login", "application/json",
		bytes.NewReader(body))
	defer resp2.Body.Close()
	json.NewDecoder(resp2.Body).Decode(&out)
	return out["token"].(string)
}
