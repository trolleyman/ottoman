package controller

import (
	"bytes"
	"io"
	"net/http"

	"github.com/trolleyman/ottoman/internal/clipboard"
)

func (c *Controller) handleImageClipboard(w http.ResponseWriter, r *http.Request) {
	// Bound the body here too, before forwarding anything to the desktop.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, clipboard.MaxImageBytes))
	if err != nil {
		http.Error(w, "Image must be smaller than 20 MB", http.StatusRequestEntityTooLarge)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, c.agentURL("/api/clipboard/image"), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "Could not create image upload", http.StatusBadGateway)
		return
	}
	req.Header.Set("Content-Type", "image/png")
	if c.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.AuthToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		http.Error(w, "Could not reach the desktop to copy the image", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, io.LimitReader(resp.Body, 4096))
}
