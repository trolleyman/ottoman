//go:build linux

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/coder/websocket"
	"github.com/trolleyman/ottoman/internal/common"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/config"
	"github.com/trolleyman/ottoman/internal/input"
	"github.com/trolleyman/ottoman/internal/store"
	"github.com/trolleyman/ottoman/internal/textcontext"
)

func TestGreeterStartupDoesNotRestoreSavedTVLayout(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_SESSION_TYPE", "x11")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("PATH", dir)
	marker := filepath.Join(dir, "display-command")
	t.Setenv("OTTOMAN_TEST_DISPLAY_COMMAND", marker)
	// The old startup path would query xrandr and then attempt the saved TV
	// layout. Neither probing nor applying a layout belongs in construction.
	if err := os.WriteFile(filepath.Join(dir, "xrandr"), []byte("#!/bin/sh\necho called >> \"$OTTOMAN_TEST_DISPLAY_COMMAND\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCurrentLayout("tv"); err != nil {
		t.Fatal(err)
	}
	a, err := newAgent(&config.AgentConfig{
		ListenAddress: "127.0.0.1:0", AuthToken: "test",
		Layouts: []api.Layout{{Id: "tv", Monitors: []api.LayoutMonitor{{Port: "HDMI-2", Width: 3840, Height: 2160}}}},
	}, true, func() (input.MouseController, input.KeyboardController, error) {
		return input.NewSimulatedMouse(0, 0, -100, -100, 100, 100), &greeterTestKeyboard{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.mouse == nil || a.keyboard == nil || a.audio != nil {
		t.Fatal("greeter must enable input and leave audio disabled")
	}
	if _, err := a.textContextBackend(context.Background()); !errors.Is(err, textcontext.ErrUnavailable) {
		t.Fatal("greeter attempted accessible text mirroring")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("startup invoked a display command: %v", err)
	}
	if a.currentLayout != "" {
		t.Fatalf("startup claimed saved layout %q was active", a.currentLayout)
	}
	if got := store.LoadCurrentLayout(); got != "tv" {
		t.Fatalf("startup rewrote saved choice to %q", got)
	}
}

type greeterTestKeyboard struct{ pressed chan string }

func (k *greeterTestKeyboard) KeyDown(key string, _ []string) error {
	if k.pressed != nil {
		k.pressed <- key
	}
	return nil
}
func (*greeterTestKeyboard) KeyUp(string, []string) error { return nil }

func TestGreeterAuthenticatedTrackpadAndKeyboardWithoutTextMirroring(t *testing.T) {
	keyboard := &greeterTestKeyboard{pressed: make(chan string, 4)}
	mouse := input.NewSimulatedMouse(0, 0, -100, -100, 100, 100)
	a := &Agent{mouse: mouse, keyboard: keyboard, greeter: true}
	auth := common.NewAuthenticator("greeter-test-token", true)
	server := httptest.NewServer(auth.Middleware(http.HandlerFunc(a.handleTrackpad)))
	defer server.Close()
	response, err := http.Get(server.URL + "/api/trackpad")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("login-screen input did not require authentication")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/trackpad", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": []string{"Bearer greeter-test-token"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if _, _, err = conn.Read(ctx); err != nil {
		t.Fatal(err)
	}
	send := func(value interface{}) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = conn.Write(ctx, websocket.MessageText, data); err != nil {
			t.Fatal(err)
		}
	}
	send(map[string]interface{}{"type": "textwatch", "enabled": true})
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var v api.TrackpadMessageTextContext
		if json.Unmarshal(data, &v) != nil || v.Type != api.Textcontext {
			continue
		}
		if !v.Protected || v.Available || v.Password || v.MaskedLength != nil || v.Value != "" {
			t.Fatal("greeter allowed native text mirroring")
		}
		break
	}
	send(map[string]interface{}{"type": "mousemoverel", "dx": 5, "dy": 7})
	send(map[string]interface{}{"type": "keydown", "key": "Enter"})
	select {
	case key := <-keyboard.pressed:
		if key != "Enter" {
			t.Fatal("wrong key injected")
		}
	case <-ctx.Done():
		t.Fatal("greeter keyboard did not receive input")
	}
	x, y, err := mouse.GetPosition()
	if err != nil || x != 5 || y != 7 {
		t.Fatal("greeter trackpad did not move the cursor")
	}
	send(map[string]interface{}{"type": "keyup", "key": "Enter"})
}

func TestGreeterUnavailableInputKeepsLayoutAgentRunning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_SESSION_TYPE", "x11")
	t.Setenv("WAYLAND_DISPLAY", "")
	a, err := newAgent(&config.AgentConfig{ListenAddress: "127.0.0.1:0", AuthToken: "test"}, true, func() (input.MouseController, input.KeyboardController, error) {
		return nil, nil, errors.New("uinput unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.displayMgr == nil || a.mouse != nil || a.keyboard != nil || a.audio != nil {
		t.Fatal("missing uinput must leave display-only greeter service available")
	}
	response := httptest.NewRecorder()
	a.handleTrackpad(response, httptest.NewRequest(http.MethodGet, "/api/trackpad", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatal("greeter claimed unavailable input was ready")
	}
}
