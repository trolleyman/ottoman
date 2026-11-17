package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestAlternateAgentRoutesHTTPAndWebSocket(t *testing.T) {
	status := http.StatusOK
	alternate := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/health" {
			t.Errorf("probe path: %s", r.URL.Path)
		}
		w.WriteHeader(status)
	}))
	defer alternate.Close()
	alt, err := url.Parse(alternate.URL + "/agent")
	if err != nil {
		t.Fatal(err)
	}
	primary, err := url.Parse("http://linux.example")
	if err != nil {
		t.Fatal(err)
	}
	c := &Controller{client: alternate.Client(), agentBase: primary, alternateBase: alt}
	if got := c.agentURL("/api/layouts"); got != alternate.URL+"/agent/api/layouts" {
		t.Fatal(got)
	}
	if got := c.agentWebSocketURL("/api/trackpad"); got != "wss://"+alt.Host+"/agent/api/trackpad" {
		t.Fatal(got)
	}
	status = http.StatusServiceUnavailable
	c.nextProbe = time.Time{}
	if got := c.agentURL("/api/layouts"); got != "http://linux.example/api/layouts" {
		t.Fatal(got)
	}
	if got := c.agentWebSocketURL("/api/trackpad"); got != "ws://linux.example/api/trackpad" {
		t.Fatal(got)
	}
}
