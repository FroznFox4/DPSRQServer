package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/kirill/gamelogserver/internal/model"
	"github.com/kirill/gamelogserver/internal/service"
)

type leaderboardServiceIface interface {
	GetLeaderboard(ctx context.Context, category string, limit int) ([]model.LeaderboardEntry, error)
}

type LeaderboardHandler struct {
	lbService leaderboardServiceIface
}

func NewLeaderboardHandler(lbService *service.LeaderboardService) *LeaderboardHandler {
	return &LeaderboardHandler{lbService: lbService}
}

func (h *LeaderboardHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	category := chi.URLParam(r, "category")
	if category == "" {
		writeError(w, "category is required", http.StatusBadRequest)
		return
	}

	limit := 10
	if q := r.URL.Query().Get("limit"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}

	entries, err := h.lbService.GetLeaderboard(r.Context(), category, limit)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, entries)
}
