package agent

import (
	"net/http/httptest"
	"testing"
)

func TestArtworkUnavailableOnLoginScreen(t *testing.T) {
	a := &Agent{greeter: true}
	out := httptest.NewRecorder()
	a.handleMediaArtwork(out, httptest.NewRequest("GET", "/api/media/artwork?key=example", nil))
	if out.Code != 404 || out.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("greeter artwork must be unavailable and uncached")
	}
}
