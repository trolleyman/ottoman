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

func (a *Agent) handleTextClipboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if a.greeter {
		http.Error(w, "Clipboard sharing is unavailable on the login screen", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if r.Method == http.MethodGet {
		value, err := clipboard.ReadText(ctx)
		if err != nil {
			http.Error(w, "Could not read desktop clipboard. Check the desktop clipboard tools.", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(value))
		return
	}
	value, status, err := clipboard.ReadTextBody(w, r)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	if err := clipboard.WriteText(ctx, string(value)); err != nil {
		http.Error(w, "Could not set desktop clipboard. Check the desktop clipboard tools.", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
