package service

import (
	"context"
	"fmt"

	"github.com/kirill/gamelogserver/internal/model"
	"github.com/kirill/gamelogserver/internal/repository"
)

const maxBatchSize = 1000

// gameLogRepository is the subset of *repository.GameLogRepo used by SyncService.
type gameLogRepository interface {
	BatchInsert(ctx context.Context, userID int, logs []model.SyncLogEntry) (inserted, duplicates int, err error)
}

type SyncService struct {
	gameLogRepo gameLogRepository
}

func NewSyncService(gameLogRepo *repository.GameLogRepo) *SyncService {
	return &SyncService{gameLogRepo: gameLogRepo}
}

func (s *SyncService) SyncLogs(ctx context.Context, userID int, req *model.SyncRequest) (*model.SyncResponse, error) {
	if len(req.Logs) == 0 {
		return &model.SyncResponse{}, nil
	}
	if len(req.Logs) > maxBatchSize {
		return nil, fmt.Errorf("batch too large: max %d entries", maxBatchSize)
	}

	inserted, duplicates, err := s.gameLogRepo.BatchInsert(ctx, userID, req.Logs)
	if err != nil {
		return nil, fmt.Errorf("batch insert: %w", err)
	}

	return &model.SyncResponse{
		Synced:     inserted,
		Duplicates: duplicates,
	}, nil
}
