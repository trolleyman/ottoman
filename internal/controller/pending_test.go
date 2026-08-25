package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/config"
)

func TestPendingLayoutArmAndClear(t *testing.T) {
	var p pendingLayout
	now := time.Now()

	if got := p.get(now); got != "" {
		t.Errorf("get on a fresh pendingLayout = %q, want empty", got)
	}
	if started := p.arm("tv", now); !started {
		t.Error("arm on an idle pendingLayout said not to start the orchestrator")
	}
	if got := p.get(now); got != "tv" {
		t.Errorf("get = %q, want tv", got)
	}

	// A second choice replaces the first without starting a second orchestrator,
	// or two goroutines would race to apply different layouts.
	if started := p.arm("desk", now); started {
		t.Error("arm while running said to start another orchestrator")
	}
	if got := p.get(now); got != "desk" {
		t.Errorf("get = %q, want the replacement", got)
	}

	p.clear()
	if got := p.get(now); got != "" {
		t.Errorf("get after clear = %q, want empty", got)
	}
	if started := p.arm("tv", now); !started {
		t.Error("arm after clear did not restart the orchestrator")
	}
}

func TestPendingLayoutExpires(t *testing.T) {
	var p pendingLayout
	now := time.Now()
	p.arm("tv", now)

	if got := p.get(now.Add(pendingTTL - time.Second)); got != "tv" {
		t.Errorf("get just inside the TTL = %q, want tv", got)
	}
	// A choice made and forgotten must not hijack a boot hours later.
	if got := p.get(now.Add(pendingTTL + time.Second)); got != "" {
		t.Errorf("get past the TTL = %q, want empty", got)
	}
}

func TestPendingLayoutGreeterHandover(t *testing.T) {
	var p pendingLayout
	p.arm("tv", time.Now())

	// The login screen gets it once...
	if !p.needsApply(true) {
		t.Error("needsApply(greeter) = false before the greeter had it")
	}
	p.markGreeterApplied()
	if p.needsApply(true) {
		t.Error("needsApply(greeter) = true again; the greeter would be re-applied on every poll")
	}
	// ...and the session gets it too, because Mutter restores the session's own
	// layout on login regardless of what the login screen was showing.
	if !p.needsApply(false) {
		t.Error("needsApply(session) = false after the greeter; the session would come up on the wrong display")
	}
}

// testControllerConfig is a minimal valid controller config aimed at agentURL.
func testControllerConfig(agentURL string) *config.ControllerConfig {
	return &config.ControllerConfig{
		ListenAddress: "127.0.0.1:0",
		AuthToken:     "0123456789abcdef0123",
		Agent:         config.AgentControllerConfig{URL: agentURL},
	}
}

// newTestController builds a controller pointed at agentURL, with its data dir
// redirected into the test's temp dir.
func newTestController(t *testing.T, agentURL string) *Controller {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	c, err := New(testControllerConfig(agentURL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestSwitchLayoutQueuesWhenAgentIsDown(t *testing.T) {
	// A port with nothing on it: the switch can't be delivered.
	c := newTestController(t, "http://127.0.0.1:1")

	resp, err := c.SwitchLayout(t.Context(), api.SwitchLayoutRequestObject{
		Body: &api.SwitchLayoutJSONRequestBody{Layout: "tv"},
	})
	if err != nil {
		t.Fatalf("SwitchLayout: %v", err)
	}
	ok, isOK := resp.(api.SwitchLayout200JSONResponse)
	if !isOK {
		t.Fatalf("SwitchLayout returned %T, want a queued 200", resp)
	}
	if ok.Queued == nil || !*ok.Queued {
		t.Error("response is not marked queued, so the UI would report a plain success")
	}
	if got := c.PendingLayout(); got != "tv" {
		t.Errorf("PendingLayout = %q, want tv", got)
	}
}

func TestGetLayoutsFallsBackToTheMirror(t *testing.T) {
	c := newTestController(t, "http://127.0.0.1:1")

	// Nothing mirrored yet: an unreachable agent is a plain failure.
	resp, err := c.GetLayouts(t.Context(), api.GetLayoutsRequestObject{})
	if err != nil {
		t.Fatalf("GetLayouts: %v", err)
	}
	if _, bad := resp.(api.GetLayouts502JSONResponse); !bad {
		t.Fatalf("GetLayouts with no mirror returned %T, want 502", resp)
	}

	// With a mirror, the list survives the desktop going down - which is when
	// you need it, to choose where it should come back up.
	writeTestMirror(t, api.LayoutsResponse{
		Layouts:       []api.Layout{{Id: "tv", Name: "TV"}},
		CurrentLayout: "desk",
	})
	c.queueLayout("tv")

	resp, err = c.GetLayouts(t.Context(), api.GetLayoutsRequestObject{})
	if err != nil {
		t.Fatalf("GetLayouts: %v", err)
	}
	got, okResp := resp.(api.GetLayouts200JSONResponse)
	if !okResp {
		t.Fatalf("GetLayouts returned %T, want the mirrored 200", resp)
	}
	if len(got.Layouts) != 1 || got.Layouts[0].Id != "tv" {
		t.Errorf("layouts = %+v, want the mirrored one", got.Layouts)
	}
	if got.PendingLayout == nil || *got.PendingLayout != "tv" {
		t.Errorf("pending_layout = %v, want tv so the UI can show the queued choice", got.PendingLayout)
	}
}

func TestGetLayoutsReportsPendingFromTheAgent(t *testing.T) {
	// A live agent's layouts must still carry the queued choice, or the UI
	// would drop the "queued" badge the moment the desktop answered.
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/layouts" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.LayoutsResponse{
			Layouts:       []api.Layout{{Id: "desk", Name: "Desk"}},
			CurrentLayout: "desk",
		})
	}))
	defer agent.Close()

	c := newTestController(t, agent.URL)
	c.queueLayout("tv")

	resp, err := c.GetLayouts(t.Context(), api.GetLayoutsRequestObject{})
	if err != nil {
		t.Fatalf("GetLayouts: %v", err)
	}
	got, okResp := resp.(api.GetLayouts200JSONResponse)
	if !okResp {
		t.Fatalf("GetLayouts returned %T, want 200", resp)
	}
	if len(got.Layouts) != 1 || got.Layouts[0].Id != "desk" {
		t.Errorf("layouts = %+v, want the agent's", got.Layouts)
	}
	if got.PendingLayout == nil || *got.PendingLayout != "tv" {
		t.Errorf("pending_layout = %v, want tv", got.PendingLayout)
	}
}

func TestLayoutsMirrorRoundTrip(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if _, ok := mirroredLayouts(); ok {
		t.Error("mirroredLayouts reported a mirror before one was written")
	}
	writeTestMirror(t, api.LayoutsResponse{
		Layouts:       []api.Layout{{Id: "tv", Name: "TV"}, {Id: "desk", Name: "Desk"}},
		CurrentLayout: "tv",
	})

	got, ok := mirroredLayouts()
	if !ok {
		t.Fatal("mirroredLayouts found nothing after a write")
	}
	if len(got.Layouts) != 2 || got.CurrentLayout != "tv" {
		t.Errorf("mirror = %+v, want both layouts and current tv", got)
	}
}

func TestLayoutsMirrorUsesCachedAgentExport(t *testing.T) {
	requested := make(chan string, 1)
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.LayoutsResponse{
			Layouts:       []api.Layout{{Id: "tv", Name: "TV"}},
			CurrentLayout: "tv",
		})
	}))
	defer agent.Close()

	c := newTestController(t, agent.URL)
	if err := c.syncLayoutsFromAgent(t.Context()); err != nil {
		t.Fatalf("syncLayoutsFromAgent: %v", err)
	}
	if path := <-requested; path != "/api/layouts/export" {
		t.Fatalf("mirror requested %q, want cached export", path)
	}
	if got, ok := mirroredLayouts(); !ok || got.CurrentLayout != "tv" || len(got.Layouts) != 1 {
		t.Fatalf("mirror = %+v, %v; want exported layouts", got, ok)
	}
}

// writeTestMirror puts a layouts mirror on disk the way syncLayoutsFromAgent
// would.
func writeTestMirror(t *testing.T, l api.LayoutsResponse) {
	t.Helper()
	body, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("marshalling mirror: %v", err)
	}
	if err := os.MkdirAll(layoutsMirrorDir(), 0o755); err != nil {
		t.Fatalf("creating data dir: %v", err)
	}
	if err := os.WriteFile(layoutsMirrorPath(), body, 0o600); err != nil {
		t.Fatalf("writing mirror: %v", err)
	}
}

// fakeAgent serves just enough of the agent API for the queued-layout
// orchestrator: a status that says whether this is the login screen, and a
// layout switch that records what it was asked for.
type fakeAgent struct {
	mu       sync.Mutex
	greeter  bool
	switches []string
}

func (f *fakeAgent) setGreeter(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.greeter = v
}

func (f *fakeAgent) applied() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.switches...)
}

func (f *fakeAgent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	greeter := f.greeter
	f.mu.Unlock()

	switch r.URL.Path {
	case "/api/status":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.StatusResponse{Status: "ok", Greeter: &greeter})
	case "/api/layouts/switch":
		var body api.SwitchLayoutRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.switches = append(f.switches, body.Layout)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SwitchLayoutResponse{Success: true, CurrentLayout: body.Layout})
	default:
		http.NotFound(w, r)
	}
}

func TestQueuedLayoutAppliesToTheLoginScreenThenTheSession(t *testing.T) {
	agent := &fakeAgent{greeter: true}
	srv := httptest.NewServer(agent)
	defer srv.Close()

	c := newTestController(t, srv.URL)
	c.pendingPollInterval = 5 * time.Millisecond // set before anything is armed
	c.queueLayout("tv")

	// The login screen gets it first, so you can see the machine boot on the
	// display you asked for...
	waitFor(t, func() bool { return len(agent.applied()) == 1 })

	// ...and the choice stays armed, because logging in hands the display back
	// to the session's own Mutter config.
	if got := c.PendingLayout(); got != "tv" {
		t.Errorf("PendingLayout after the greeter = %q, want it still armed for the session", got)
	}
	// The greeter must not be re-applied on every poll while we wait.
	time.Sleep(50 * time.Millisecond)
	if got := agent.applied(); len(got) != 1 {
		t.Errorf("applied %v to the login screen, want exactly one", got)
	}

	// The session agent takes over: it gets the layout too, and that clears it.
	agent.setGreeter(false)
	waitFor(t, func() bool { return len(agent.applied()) == 2 })
	waitFor(t, func() bool { return c.PendingLayout() == "" })

	if got := agent.applied(); got[0] != "tv" || got[1] != "tv" {
		t.Errorf("applied %v, want tv to both the login screen and the session", got)
	}
}

// waitFor polls cond until it holds, failing the test if it never does.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition never held")
}
