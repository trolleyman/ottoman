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

func (c *Controller) handleTextClipboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var body []byte
	if r.Method == http.MethodPost {
		var status int
		var err error
		body, status, err = clipboard.ReadTextBody(w, r)
		if err != nil {
			http.Error(w, err.Error(), status)
			return
		}
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, c.agentURL("/api/clipboard/text"), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "Could not create clipboard request", http.StatusBadGateway)
		return
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if c.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.AuthToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		http.Error(w, "Desktop clipboard is unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, clipboard.MaxTextBytes+1))
	if err != nil || len(data) > clipboard.MaxTextBytes {
		http.Error(w, "Desktop clipboard text is too large or unavailable", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(resp.StatusCode)
	w.Write(data)
}
