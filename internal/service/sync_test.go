package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/kirill/gamelogserver/internal/model"
)

// mockGameLogRepo implements gameLogRepository for SyncService tests.
type mockGameLogRepo struct {
	inserted   int
	duplicates int
	err        error
}

func (m *mockGameLogRepo) BatchInsert(_ context.Context, _ int, _ []model.SyncLogEntry) (int, int, error) {
	if m.err != nil {
		return 0, 0, m.err
	}
	return m.inserted, m.duplicates, nil
}

func newSyncService(mock *mockGameLogRepo) *SyncService {
	return &SyncService{gameLogRepo: mock}
}

func makeLogs(n int) []model.SyncLogEntry {
	logs := make([]model.SyncLogEntry, n)
	for i := range logs {
		logs[i] = model.SyncLogEntry{ClientID: i + 1, Timestamp: "2026-03-13T12:00:00Z", Kind: "gold"}
	}
	return logs
}

func TestSyncService_EmptyBatch_ReturnsZeros(t *testing.T) {
	svc := newSyncService(&mockGameLogRepo{})
	resp, err := svc.SyncLogs(context.Background(), 1, &model.SyncRequest{Logs: nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Synced != 0 || resp.Duplicates != 0 {
		t.Errorf("expected zeros, got %+v", resp)
	}
}

func TestSyncService_BatchTooLarge_ReturnsError(t *testing.T) {
	svc := newSyncService(&mockGameLogRepo{})
	_, err := svc.SyncLogs(context.Background(), 1, &model.SyncRequest{Logs: makeLogs(maxBatchSize + 1)})
	if err == nil {
		t.Fatal("expected error for batch > maxBatchSize")
	}
	want := fmt.Sprintf("batch too large: max %d entries", maxBatchSize)
	if err.Error() != want {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestSyncService_ExactMaxBatch_Accepted(t *testing.T) {
	svc := newSyncService(&mockGameLogRepo{inserted: maxBatchSize})
	resp, err := svc.SyncLogs(context.Background(), 1, &model.SyncRequest{Logs: makeLogs(maxBatchSize)})
	if err != nil {
		t.Fatalf("unexpected error for exactly maxBatchSize: %v", err)
	}
	if resp.Synced != maxBatchSize {
		t.Errorf("expected Synced=%d, got %d", maxBatchSize, resp.Synced)
	}
}

func TestSyncService_RepoError_Propagated(t *testing.T) {
	svc := newSyncService(&mockGameLogRepo{err: fmt.Errorf("connection refused")})
	_, err := svc.SyncLogs(context.Background(), 1, &model.SyncRequest{Logs: makeLogs(1)})
	if err == nil {
		t.Fatal("expected error from repo to propagate")
	}
}

func TestSyncService_DuplicatesReported(t *testing.T) {
	svc := newSyncService(&mockGameLogRepo{inserted: 3, duplicates: 2})
	resp, err := svc.SyncLogs(context.Background(), 1, &model.SyncRequest{Logs: makeLogs(5)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Synced != 3 {
		t.Errorf("expected Synced=3, got %d", resp.Synced)
	}
	if resp.Duplicates != 2 {
		t.Errorf("expected Duplicates=2, got %d", resp.Duplicates)
	}
}
