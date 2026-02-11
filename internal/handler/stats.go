package handler

import (
	"net/http"

	"github.com/kirill/gamelogserver/internal/middleware"
	"github.com/kirill/gamelogserver/internal/service"
)

type StatsHandler struct {
	statsService *service.StatsService
}

func NewStatsHandler(statsService *service.StatsService) *StatsHandler {
	return &StatsHandler{statsService: statsService}
}

func (h *StatsHandler) GetMyStats(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == 0 {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	stats, err := h.statsService.GetUserStats(r.Context(), userID)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
