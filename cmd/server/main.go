package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/kirill/gamelogserver/internal/config"
	"github.com/kirill/gamelogserver/internal/handler"
	"github.com/kirill/gamelogserver/internal/middleware"
	"github.com/kirill/gamelogserver/internal/partysync"
	pb "github.com/kirill/gamelogserver/internal/partysync/pb"
	"github.com/kirill/gamelogserver/internal/repository"
	"github.com/kirill/gamelogserver/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("database ping: %v", err)
	}
	log.Println("connected to database")

	// Repositories
	userRepo := repository.NewUserRepo(pool)
	gameLogRepo := repository.NewGameLogRepo(pool)
	lbRepo := repository.NewLeaderboardRepo(pool)

	// Services
	authService := service.NewAuthService(userRepo, cfg.JWTSecret)
	syncService := service.NewSyncService(gameLogRepo)
	lbService := service.NewLeaderboardService(lbRepo)
	statsService := service.NewStatsService(gameLogRepo)

	// Start leaderboard refresh (every 60s)
	lbService.StartRefresh(ctx, 60*time.Second)

	// Handlers
	authHandler := handler.NewAuthHandler(authService)
	syncHandler := handler.NewSyncHandler(syncService)
	lbHandler := handler.NewLeaderboardHandler(lbService)
	statsHandler := handler.NewStatsHandler(statsService)

	// Router
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)

	r.Route("/api/v1", func(r chi.Router) {
		// Auth (public)
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)

		// Leaderboard (public)
		r.Get("/leaderboard/{category}", lbHandler.GetLeaderboard)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.JWTAuth(authService))
			r.Post("/logs/sync", syncHandler.SyncLogs)
			r.Get("/stats/me", statsHandler.GetMyStats)
		})
	})

	// Server
	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// gRPC server on :50051
	grpcSrv := grpc.NewServer(
		grpc.StreamInterceptor(authStreamInterceptor(authService)),
	)
	hub := partysync.NewHub()
	pb.RegisterPartySyncServer(grpcSrv, partysync.NewService(hub, authService))

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		cancel()

		grpcSrv.GracefulStop()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	grpcLis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("grpc listen: %v", err)
	}
	go func() {
		log.Println("gRPC server starting on :50051")
		if err := grpcSrv.Serve(grpcLis); err != nil {
			log.Printf("gRPC server stopped: %v", err)
		}
	}()

	log.Printf("server starting on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
	log.Println("server stopped")
}

func authStreamInterceptor(authSvc *service.AuthService) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			return fmt.Errorf("missing metadata")
		}
		tokens := md.Get("authorization")
		if len(tokens) == 0 {
			return fmt.Errorf("missing authorization")
		}
		tokenStr := strings.TrimPrefix(tokens[0], "Bearer ")
		if _, _, err := authSvc.ValidateToken(tokenStr); err != nil {
			return fmt.Errorf("invalid token: %w", err)
		}
		return handler(srv, ss)
	}
}
