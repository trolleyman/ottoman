package controller

import (
	"bytes"
	"context"
	"encoding/hex"
	"github.com/trolleyman/ottoman/internal/media"
	"io"
	"net/http"
	"time"
)

func (c *Controller) handleMedia(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		http.Error(w, "Media command is too large", http.StatusRequestEntityTooLarge)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, c.agentURL("/api/media"), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "Media unavailable", http.StatusBadGateway)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.AuthToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"available":false}`))
			return
		}
		http.Error(w, "Desktop unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, io.LimitReader(resp.Body, 8192))
}

func (c *Controller) handleMediaArtwork(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	key := r.URL.Query().Get("key")
	if len(key) != 64 {
		http.NotFound(w, r)
		return
	}
	if _, err := hex.DecodeString(key); err != nil {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.agentURL("/api/media/artwork")+"?key="+key, nil)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if c.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.AuthToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" {
		http.NotFound(w, r)
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, media.MaxArtworkBytes+1))
	if err != nil || len(data) > media.MaxArtworkBytes {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(data)
}
