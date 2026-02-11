package service

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/kirill/gamelogserver/internal/model"
	"github.com/kirill/gamelogserver/internal/repository"
)

type LeaderboardService struct {
	repo  *repository.LeaderboardRepo
	mu    sync.RWMutex
	cache map[string][]model.LeaderboardEntry
}

func NewLeaderboardService(repo *repository.LeaderboardRepo) *LeaderboardService {
	return &LeaderboardService{
		repo:  repo,
		cache: make(map[string][]model.LeaderboardEntry),
	}
}

func (s *LeaderboardService) GetLeaderboard(ctx context.Context, category string, limit int) ([]model.LeaderboardEntry, error) {
	s.mu.RLock()
	cached, ok := s.cache[category]
	s.mu.RUnlock()

	if ok && len(cached) > 0 {
		if limit > len(cached) {
			limit = len(cached)
		}
		return cached[:limit], nil
	}

	// Cache miss — query directly
	return s.repo.GetLeaderboard(ctx, category, limit)
}

// StartRefresh launches a background goroutine that refreshes the materialized view
// and repopulates the cache every interval.
func (s *LeaderboardService) StartRefresh(ctx context.Context, interval time.Duration) {
	go func() {
		// Initial load
		s.refresh(ctx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refresh(ctx)
			}
		}
	}()
}

func (s *LeaderboardService) refresh(ctx context.Context) {
	if err := s.repo.RefreshView(ctx); err != nil {
		log.Printf("leaderboard: refresh view error: %v", err)
		return
	}

	categories := []string{"exp", "gold", "kills", "items"}
	newCache := make(map[string][]model.LeaderboardEntry, len(categories))

	for _, cat := range categories {
		entries, err := s.repo.GetLeaderboard(ctx, cat, 100)
		if err != nil {
			log.Printf("leaderboard: query %s error: %v", cat, err)
			continue
		}
		newCache[cat] = entries
	}

	s.mu.Lock()
	s.cache = newCache
	s.mu.Unlock()
}
