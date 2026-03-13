package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kirill/gamelogserver/internal/model"
)

type mockStatsService struct {
	fn func(ctx context.Context, userID int) (*model.UserStats, error)
}

func (m *mockStatsService) GetUserStats(ctx context.Context, userID int) (*model.UserStats, error) {
	return m.fn(ctx, userID)
}

func TestStatsHandler_Success(t *testing.T) {
	expected := &model.UserStats{
		TotalMonsters: 10,
		TotalExp:      5000,
		TotalGold:     1200,
		TotalItems:    7,
		Monsters: []model.NameCountPair{
			{Name: "Dragon", Count: 5},
			{Name: "Orc", Count: 5},
		},
		Items: []model.NameCountPair{
			{Name: "Iron Ore", Count: 7},
		},
	}
	svc := &mockStatsService{
		fn: func(_ context.Context, userID int) (*model.UserStats, error) {
			if userID != 3 {
				t.Errorf("expected userID=3, got %d", userID)
			}
			return expected, nil
		},
	}
	h := &StatsHandler{statsService: svc}

	req := withUserID(httptest.NewRequest(http.MethodGet, "/stats/me", nil), 3)
	rr := httptest.NewRecorder()
	h.GetMyStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var got model.UserStats
	json.NewDecoder(rr.Body).Decode(&got)
	if got.TotalMonsters != 10 {
		t.Errorf("expected TotalMonsters=10, got %d", got.TotalMonsters)
	}
	if got.TotalExp != 5000 {
		t.Errorf("expected TotalExp=5000, got %d", got.TotalExp)
	}
}

func TestStatsHandler_NoUserID_Returns401(t *testing.T) {
	h := &StatsHandler{statsService: &mockStatsService{}}
	req := httptest.NewRequest(http.MethodGet, "/stats/me", nil)
	rr := httptest.NewRecorder()
	h.GetMyStats(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestStatsHandler_ServiceError_Returns500(t *testing.T) {
	svc := &mockStatsService{
		fn: func(_ context.Context, _ int) (*model.UserStats, error) {
			return nil, errors.New("db connection lost")
		},
	}
	h := &StatsHandler{statsService: svc}

	req := withUserID(httptest.NewRequest(http.MethodGet, "/stats/me", nil), 1)
	rr := httptest.NewRecorder()
	h.GetMyStats(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestStatsHandler_ResponseContentType(t *testing.T) {
	svc := &mockStatsService{
		fn: func(_ context.Context, _ int) (*model.UserStats, error) {
			return &model.UserStats{}, nil
		},
	}
	req := withUserID(httptest.NewRequest(http.MethodGet, "/stats/me", nil), 1)
	rr := httptest.NewRecorder()
	(&StatsHandler{statsService: svc}).GetMyStats(rr, req)

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}
