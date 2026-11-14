package agent

import (
	"context"
	"net/http"
	"time"

	"github.com/trolleyman/ottoman/internal/clipboard"
)

func (a *Agent) handleImageClipboard(w http.ResponseWriter, r *http.Request) {
	if a.greeter {
		http.Error(w, "Image paste is unavailable on the login screen", http.StatusServiceUnavailable)
		return
	}
	png, status, err := clipboard.ReadPNG(w, r)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if err := clipboard.WriteImage(ctx, png); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
