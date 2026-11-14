package controller

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/trolleyman/ottoman/internal/clipboard"
	"github.com/trolleyman/ottoman/internal/common"
	"github.com/trolleyman/ottoman/internal/config"
)

func TestImageClipboardProxy(t *testing.T) {
	calls := 0
	desktop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/desktop/api/clipboard/image" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("incorrect agent path or credentials")
		}
		body, _ := io.ReadAll(r.Body)
		if !bytes.Equal(body, []byte("image data")) {
			t.Error("upload changed")
		}
		http.Error(w, "clipboard unavailable", http.StatusServiceUnavailable)
	}))
	defer desktop.Close()
	base, _ := url.Parse(desktop.URL + "/desktop")
	c := &Controller{agentBase: base, client: desktop.Client(), config: &config.ControllerConfig{AuthToken: "secret"}}
	handler := common.NewAuthenticator("secret", true).Middleware(http.HandlerFunc(c.handleImageClipboard))
	for _, tc := range []struct {
		name   string
		body   []byte
		auth   bool
		status int
	}{
		{"requires auth", []byte("image data"), false, http.StatusUnauthorized},
		{"forwards errors", []byte("image data"), true, http.StatusServiceUnavailable},
		{"bounds upload", make([]byte, clipboard.MaxImageBytes+1), true, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/clipboard/image", bytes.NewReader(tc.body))
			if tc.auth {
				req.Header.Set("Authorization", "Bearer secret")
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.status == http.StatusServiceUnavailable && w.Body.String() != "clipboard unavailable\n" {
				t.Fatal("agent error lost")
			}
		})
	}
	if calls != 1 {
		t.Fatalf("agent called %d times; want 1", calls)
	}
}
