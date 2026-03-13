package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kirill/gamelogserver/internal/middleware"
	"github.com/kirill/gamelogserver/internal/model"
)

type mockSyncService struct {
	fn func(ctx context.Context, userID int, req *model.SyncRequest) (*model.SyncResponse, error)
}

func (m *mockSyncService) SyncLogs(ctx context.Context, userID int, req *model.SyncRequest) (*model.SyncResponse, error) {
	return m.fn(ctx, userID, req)
}

func withUserID(r *http.Request, id int) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserIDKey, id)
	return r.WithContext(ctx)
}

func TestSyncHandler_Success(t *testing.T) {
	svc := &mockSyncService{
		fn: func(_ context.Context, _ int, req *model.SyncRequest) (*model.SyncResponse, error) {
			return &model.SyncResponse{Synced: len(req.Logs), Duplicates: 0}, nil
		},
	}
	h := &SyncHandler{syncService: svc}

	body, _ := json.Marshal(model.SyncRequest{Logs: []model.SyncLogEntry{
		{ClientID: 1, Timestamp: "2026-03-13T12:00:00Z", Kind: "gold"},
		{ClientID: 2, Timestamp: "2026-03-13T12:01:00Z", Kind: "monster"},
	}})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/logs/sync", bytes.NewBuffer(body)), 7)
	rr := httptest.NewRecorder()
	h.SyncLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp model.SyncResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Synced != 2 {
		t.Errorf("expected Synced=2, got %d", resp.Synced)
	}
}

func TestSyncHandler_NoUserID_Returns401(t *testing.T) {
	h := &SyncHandler{syncService: &mockSyncService{}}

	req := httptest.NewRequest(http.MethodPost, "/logs/sync", bytes.NewBufferString("{}"))
	rr := httptest.NewRecorder()
	h.SyncLogs(rr, req) // no userID in context

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestSyncHandler_InvalidJSON_Returns400(t *testing.T) {
	h := &SyncHandler{syncService: &mockSyncService{}}
	req := withUserID(httptest.NewRequest(http.MethodPost, "/logs/sync", bytes.NewBufferString("{bad")), 1)
	rr := httptest.NewRecorder()
	h.SyncLogs(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSyncHandler_ServiceError_Returns400(t *testing.T) {
	svc := &mockSyncService{
		fn: func(_ context.Context, _ int, _ *model.SyncRequest) (*model.SyncResponse, error) {
			return nil, errors.New("batch too large: max 1000 entries")
		},
	}
	h := &SyncHandler{syncService: svc}

	body, _ := json.Marshal(model.SyncRequest{Logs: []model.SyncLogEntry{{ClientID: 1, Kind: "gold"}}})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/logs/sync", bytes.NewBuffer(body)), 1)
	rr := httptest.NewRecorder()
	h.SyncLogs(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	var errResp map[string]string
	json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp["error"] == "" {
		t.Error("expected error field in response body")
	}
}

func TestSyncHandler_EmptyBatch_Returns200(t *testing.T) {
	svc := &mockSyncService{
		fn: func(_ context.Context, _ int, _ *model.SyncRequest) (*model.SyncResponse, error) {
			return &model.SyncResponse{}, nil
		},
	}
	h := &SyncHandler{syncService: svc}

	body, _ := json.Marshal(model.SyncRequest{Logs: []model.SyncLogEntry{}})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/logs/sync", bytes.NewBuffer(body)), 5)
	rr := httptest.NewRecorder()
	h.SyncLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
