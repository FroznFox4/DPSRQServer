package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/kirill/gamelogserver/internal/model"
)

type mockLeaderboardService struct {
	fn func(ctx context.Context, category string, limit int) ([]model.LeaderboardEntry, error)
}

func (m *mockLeaderboardService) GetLeaderboard(ctx context.Context, category string, limit int) ([]model.LeaderboardEntry, error) {
	return m.fn(ctx, category, limit)
}

// chiRequest creates an *http.Request with chi URL params set.
func chiRequest(method, path string, params map[string]string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestLeaderboardHandler_Success(t *testing.T) {
	entries := []model.LeaderboardEntry{
		{Rank: 1, Username: "alice", Value: 5000},
		{Rank: 2, Username: "bob", Value: 3000},
	}
	svc := &mockLeaderboardService{
		fn: func(_ context.Context, _ string, _ int) ([]model.LeaderboardEntry, error) {
			return entries, nil
		},
	}
	h := &LeaderboardHandler{lbService: svc}

	req := chiRequest(http.MethodGet, "/leaderboard/exp", map[string]string{"category": "exp"})
	rr := httptest.NewRecorder()
	h.GetLeaderboard(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var got []model.LeaderboardEntry
	json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 2 {
		t.Errorf("expected 2 entries, got %d", len(got))
	}
}

func TestLeaderboardHandler_DefaultLimit(t *testing.T) {
	var capturedLimit int
	svc := &mockLeaderboardService{
		fn: func(_ context.Context, _ string, limit int) ([]model.LeaderboardEntry, error) {
			capturedLimit = limit
			return []model.LeaderboardEntry{}, nil
		},
	}
	h := &LeaderboardHandler{lbService: svc}

	req := chiRequest(http.MethodGet, "/leaderboard/gold", map[string]string{"category": "gold"})
	rr := httptest.NewRecorder()
	h.GetLeaderboard(rr, req)

	if capturedLimit != 10 {
		t.Errorf("expected default limit=10, got %d", capturedLimit)
	}
}

func TestLeaderboardHandler_CustomLimit(t *testing.T) {
	var capturedLimit int
	svc := &mockLeaderboardService{
		fn: func(_ context.Context, _ string, limit int) ([]model.LeaderboardEntry, error) {
			capturedLimit = limit
			return []model.LeaderboardEntry{}, nil
		},
	}
	h := &LeaderboardHandler{lbService: svc}

	req := chiRequest(http.MethodGet, "/leaderboard/kills?limit=25", map[string]string{"category": "kills"})
	req.URL.RawQuery = "limit=25"
	rr := httptest.NewRecorder()
	h.GetLeaderboard(rr, req)

	if capturedLimit != 25 {
		t.Errorf("expected limit=25, got %d", capturedLimit)
	}
}

func TestLeaderboardHandler_InvalidLimit_UsesDefault(t *testing.T) {
	var capturedLimit int
	svc := &mockLeaderboardService{
		fn: func(_ context.Context, _ string, limit int) ([]model.LeaderboardEntry, error) {
			capturedLimit = limit
			return []model.LeaderboardEntry{}, nil
		},
	}
	h := &LeaderboardHandler{lbService: svc}

	tests := []string{"0", "-5", "101", "abc"}
	for _, lim := range tests {
		req := chiRequest(http.MethodGet, "/leaderboard/exp", map[string]string{"category": "exp"})
		req.URL.RawQuery = "limit=" + lim
		rr := httptest.NewRecorder()
		h.GetLeaderboard(rr, req)
		if capturedLimit != 10 {
			t.Errorf("limit=%s: expected default 10, got %d", lim, capturedLimit)
		}
	}
}

func TestLeaderboardHandler_ServiceError_Returns400(t *testing.T) {
	svc := &mockLeaderboardService{
		fn: func(_ context.Context, _ string, _ int) ([]model.LeaderboardEntry, error) {
			return nil, errors.New("invalid category")
		},
	}
	h := &LeaderboardHandler{lbService: svc}

	req := chiRequest(http.MethodGet, "/leaderboard/bad", map[string]string{"category": "bad"})
	rr := httptest.NewRecorder()
	h.GetLeaderboard(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestLeaderboardHandler_EmptyCategory_Returns400(t *testing.T) {
	h := &LeaderboardHandler{lbService: &mockLeaderboardService{}}

	// No chi context → URLParam returns ""
	req := httptest.NewRequest(http.MethodGet, "/leaderboard/", nil)
	rr := httptest.NewRecorder()
	h.GetLeaderboard(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty category, got %d", rr.Code)
	}
}
