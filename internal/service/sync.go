package service

import (
	"context"
	"fmt"

	"github.com/kirill/gamelogserver/internal/model"
	"github.com/kirill/gamelogserver/internal/repository"
)

const maxBatchSize = 1000

type SyncService struct {
	gameLogRepo *repository.GameLogRepo
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
