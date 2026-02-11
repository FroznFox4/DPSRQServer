package handler

import (
	"encoding/json"
	"net/http"

	"github.com/kirill/gamelogserver/internal/middleware"
	"github.com/kirill/gamelogserver/internal/model"
	"github.com/kirill/gamelogserver/internal/service"
)

type SyncHandler struct {
	syncService *service.SyncService
}

func NewSyncHandler(syncService *service.SyncService) *SyncHandler {
	return &SyncHandler{syncService: syncService}
}

func (h *SyncHandler) SyncLogs(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == 0 {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req model.SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	resp, err := h.syncService.SyncLogs(r.Context(), userID, &req)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
