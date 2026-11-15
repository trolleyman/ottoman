package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/trolleyman/ottoman/internal/common"
	"github.com/trolleyman/ottoman/internal/config"
	"github.com/trolleyman/ottoman/internal/media"
)

func TestArtworkProxy(t *testing.T) {
	calls := 0
	oversized := false
	desktop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/desktop/api/media/artwork" || r.URL.Query().Get("key") != strings.Repeat("a", 64) || r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("incorrect artwork target or credentials")
		}
		w.Header().Set("Content-Type", "image/png")
		if oversized {
			w.Write(bytes.Repeat([]byte("x"), media.MaxArtworkBytes+1))
		} else {
			w.Write([]byte("png data"))
		}
	}))
	defer desktop.Close()
	base, _ := url.Parse(desktop.URL + "/desktop")
	c := &Controller{agentBase: base, client: desktop.Client(), config: &config.ControllerConfig{AuthToken: "secret"}}
	handler := common.NewAuthenticator("secret", true).Middleware(http.HandlerFunc(c.handleMediaArtwork))
	for _, tc := range []struct {
		key         string
		auth, large bool
		status      int
	}{
		{strings.Repeat("a", 64), false, false, 401},
		{"bad", true, false, 404},
		{strings.Repeat("z", 64), true, false, 404},
		{strings.Repeat("a", 64), true, false, 200},
		{strings.Repeat("a", 64), true, true, 404},
	} {
		oversized = tc.large
		req := httptest.NewRequest("GET", "/api/media/artwork?key="+tc.key, nil)
		if tc.auth {
			req.Header.Set("Authorization", "Bearer secret")
		}
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		if out.Code != tc.status {
			t.Fatalf("status %d; want %d", out.Code, tc.status)
		}
		if out.Code == 200 && (out.Body.String() != "png data" || out.Header().Get("Cache-Control") != "no-store" || out.Header().Get("X-Content-Type-Options") != "nosniff") {
			t.Fatal("incorrect image response")
		}
	}
	if calls != 2 {
		t.Fatalf("unexpected agent requests: %d", calls)
	}
}
