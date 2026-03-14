package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"github.com/kirill/gamelogserver/internal/partysync"
	pb "github.com/kirill/gamelogserver/internal/partysync/pb"
)

func main() {
	addr := getEnv("GRPC_ADDR", ":50051")

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	grpcSrv := grpc.NewServer()
	hub := partysync.NewHub()
	pb.RegisterPartySyncServer(grpcSrv, partysync.NewService(hub))

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		grpcSrv.GracefulStop()
	}()

	log.Printf("gRPC server starting on %s", addr)
	if err := grpcSrv.Serve(lis); err != nil {
		log.Fatalf("gRPC: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
