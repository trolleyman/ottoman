package controller

import (
	"bytes"
	"io"
	"net/http"
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
