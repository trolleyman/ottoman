package agent

import (
	"context"
	"encoding/json"
	"github.com/trolleyman/ottoman/internal/media"
	"net/http"
	"time"
)

func (a *Agent) handleMedia(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if r.Method == http.MethodGet {
		if a.greeter {
			json.NewEncoder(w).Encode(media.State{})
			return
		}
		st, _ := media.Read(ctx)
		json.NewEncoder(w).Encode(st)
		return
	}
	if a.greeter {
		http.Error(w, "Media unavailable on login screen", http.StatusServiceUnavailable)
		return
	}
	var command struct {
		Player string `json:"player"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&command); err != nil {
		http.Error(w, "Invalid media command", http.StatusBadRequest)
		return
	}
	if command.Action != "toggle" && command.Action != "next" && command.Action != "previous" {
		http.Error(w, "Invalid media action", http.StatusBadRequest)
		return
	}
	if err := media.Control(ctx, command.Player, command.Action); err != nil {
		http.Error(w, "Media player or action is unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
