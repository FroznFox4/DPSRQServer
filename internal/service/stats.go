package service

import (
	"context"

	"github.com/kirill/gamelogserver/internal/model"
	"github.com/kirill/gamelogserver/internal/repository"
)

// statsLogRepository is the subset of *repository.GameLogRepo used by StatsService.
type statsLogRepository interface {
	GetUserStats(ctx context.Context, userID int) (*model.UserStats, error)
}

type StatsService struct {
	gameLogRepo statsLogRepository
}

func NewStatsService(gameLogRepo *repository.GameLogRepo) *StatsService {
	return &StatsService{gameLogRepo: gameLogRepo}
}

func (s *StatsService) GetUserStats(ctx context.Context, userID int) (*model.UserStats, error) {
	return s.gameLogRepo.GetUserStats(ctx, userID)
}
