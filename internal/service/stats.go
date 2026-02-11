package service

import (
	"context"

	"github.com/kirill/gamelogserver/internal/model"
	"github.com/kirill/gamelogserver/internal/repository"
)

type StatsService struct {
	gameLogRepo *repository.GameLogRepo
}

func NewStatsService(gameLogRepo *repository.GameLogRepo) *StatsService {
	return &StatsService{gameLogRepo: gameLogRepo}
}

func (s *StatsService) GetUserStats(ctx context.Context, userID int) (*model.UserStats, error) {
	return s.gameLogRepo.GetUserStats(ctx, userID)
}
