package handler

import (
	"encoding/json"
	"net/http"

	"instant-share/id"
	"instant-share/model"
)

type hostSessionRequest struct {
	Name string `json:"name"`
}

type hostSessionResponse struct {
	SessionID string `json:"session_id"`
	Link      string `json:"link"`
}

func HostSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req hostSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	sessionID, err := id.New()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	model.CreateSession(sessionID)

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	resp := hostSessionResponse{
		SessionID: sessionID,
		Link:      scheme + "://" + r.Host + "/" + sessionID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}