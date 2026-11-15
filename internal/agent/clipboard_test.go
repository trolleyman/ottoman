package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImageClipboardRejectsUnavailableOrInvalidInput(t *testing.T) {
	// Both cases must return without calling a real desktop clipboard.
	for _, tc := range []struct {
		name    string
		greeter bool
		status  int
	}{
		{"login screen", true, http.StatusServiceUnavailable},
		{"invalid image", false, http.StatusUnsupportedMediaType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &Agent{greeter: tc.greeter}
			w := httptest.NewRecorder()
			a.handleImageClipboard(w, httptest.NewRequest(http.MethodPost, "/api/clipboard/image", strings.NewReader("not an image")))
			if w.Code != tc.status {
				t.Fatalf("status %d; want %d", w.Code, tc.status)
			}
		})
	}
}

func TestTextClipboardRejectsLoginScreen(t *testing.T) {
	a := &Agent{greeter: true}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		w := httptest.NewRecorder()
		a.handleTextClipboard(w, httptest.NewRequest(method, "/api/clipboard/text", strings.NewReader("hello")))
		if w.Code != http.StatusServiceUnavailable || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("login clipboard must remain unavailable and uncached")
		}
	}
}
