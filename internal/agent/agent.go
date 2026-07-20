package agent

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/audio"
	"github.com/trolleyman/ottoman/internal/common"
	"github.com/trolleyman/ottoman/internal/config"
	"github.com/trolleyman/ottoman/internal/display"
	"github.com/trolleyman/ottoman/internal/input"
	"github.com/trolleyman/ottoman/internal/store"
)

// Agent is the display control agent running on the desktop
type Agent struct {
	config        *config.AgentConfig
	configPath    string
	router        *http.ServeMux
	server        *http.Server
	layouts       *display.Layouts
	layoutStore   *store.LayoutStore
	registry      *store.Registry
	control       *monitorControl
	tv            *tvManager
	displayMgr    display.Manager
	mouse         input.MouseController
	keyboard      input.KeyboardController
	audio         audio.Controller
	startTime     time.Time
	currentLayout string
	greeter       bool
}

// Ensure Agent implements StrictServerInterface
var _ api.StrictServerInterface = (*Agent)(nil)

// New creates a new agent instance.
func New(cfg *config.AgentConfig) (*Agent, error) {
	return newAgent(cfg, false)
}

// NewGreeter creates an agent for the GDM login screen: it runs as the gdm user
// against the greeter's own Mutter, so it serves only display/layout control
// (input and audio are skipped) and applies the last-used layout on startup so
// the login screen mirrors the user's session.
func NewGreeter(cfg *config.AgentConfig) (*Agent, error) {
	return newAgent(cfg, true)
}

// newAgent builds an agent. In greeter mode it skips the input and audio
// controllers, which need a real user session that the gdm greeter doesn't have.
func newAgent(cfg *config.AgentConfig, greeter bool) (*Agent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	// Load layouts from the data-dir store, migrating any legacy layouts that
	// still live in the config file (agent.layouts) on first run. The config
	// file is never written back to.
	layoutStore := store.NewLayoutStore("")
	loadedLayouts, err := layoutStore.LoadWithMigration(cfg.Layouts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load layouts store")
	}
	layouts := display.NewLayoutsFromSlice(loadedLayouts)

	// Create display manager
	mgr, err := display.NewManager(layouts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create display manager")
	}

	// Input and audio need an interactive user session (uinput permissions /
	// PipeWire) that the greeter doesn't have, so greeter mode leaves them nil;
	// the corresponding endpoints report the feature as unavailable.
	var mouse input.MouseController
	var keyboard input.KeyboardController
	var audioCtl audio.Controller
	if greeter {
		log.Println("Greeter mode: input, audio and TV control are disabled")
	} else {
		mouse, err = input.NewMouseController()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create mouse controller")
		}
		keyboard, err = input.NewKeyboardController()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create keyboard controller")
		}
		// Audio control is optional: if PipeWire/wpctl isn't available the agent
		// still runs, and the audio endpoints report the feature as unavailable.
		audioCtl, err = audio.NewController()
		if err != nil {
			log.Printf("Audio control unavailable: %v", err)
			audioCtl = nil
		}
	}

	// Monitor registry (friendly names, control backends, visibility) lives in
	// the data dir alongside layouts.
	registry, err := store.NewRegistry("")
	if err != nil {
		return nil, errors.Wrap(err, "failed to load monitor registry")
	}

	// TV manager (LG webOS). Each TV's transport is resolved from its monitor
	// registry entry (backend "tv"); pairing keys live in the data dir, not
	// the config, so a config redeploy can't drop them.
	tv := newTVManager(registry, store.NewTVStore(""))
	control := newMonitorControl(registry)
	control.tv = tv

	a := &Agent{
		config:      cfg,
		configPath:  config.ConfigPath(),
		layouts:     layouts,
		layoutStore: layoutStore,
		registry:    registry,
		control:     control,
		tv:          tv,
		displayMgr:  mgr,
		mouse:       mouse,
		keyboard:    keyboard,
		audio:       audioCtl,
		startTime:   time.Now(),
		greeter:     greeter,
	}

	if err := a.setupRoutes(); err != nil {
		return nil, err
	}

	// In greeter mode, bring the login screen up in the user's last layout.
	if greeter {
		a.applyStartupLayout()
	}

	return a, nil
}

// applyStartupLayout applies the last-used layout (recorded by the user's agent
// on each switch) so the greeter mirrors the user's session. Best-effort: any
// problem just leaves the greeter's default layout in place.
func (a *Agent) applyStartupLayout() {
	id := store.LoadCurrentLayout()
	if id == "" {
		log.Println("Greeter: no last-used layout recorded; leaving display as-is")
		return
	}
	matches := a.layouts.FindByIDOrAlias(id)
	if len(matches) != 1 {
		log.Printf("Greeter: last-used layout %q not found; leaving display as-is", id)
		return
	}
	if err := a.displayMgr.ApplyLayoutConfig(matches[0]); err != nil {
		log.Printf("Greeter: failed to apply last-used layout %q: %v", id, err)
		return
	}
	a.currentLayout = matches[0].Id
	log.Printf("Greeter: applied last-used layout %q", matches[0].Name)
}

// agentHandler wraps the strict handler to allow manual handling of WebSockets
type agentHandler struct {
	api.ServerInterface
	agent *Agent
}

// ConnectTrackpad overrides the generated handler to handle WebSocket upgrades manually
func (h *agentHandler) ConnectTrackpad(w http.ResponseWriter, r *http.Request) {
	h.agent.handleTrackpad(w, r)
}

// setupRoutes configures HTTP routes
func (a *Agent) setupRoutes() error {
	a.router = http.NewServeMux()

	// Create the strict handler
	strictHandler := api.NewStrictHandler(a, nil)

	// Wrap it to handle WebSockets manually
	handler := &agentHandler{
		ServerInterface: strictHandler,
		agent:           a,
	}

	// Register generated routes
	api.HandlerWithOptions(handler, api.StdHTTPServerOptions{
		BaseRouter: a.router,
	})

	if err := common.SetupSPAHandler(a.router); err != nil {
		return errors.Wrap(err, "failed to create SPA handler")
	}

	return nil
}

// CheckHealth implements api.StrictServerInterface
func (a *Agent) CheckHealth(ctx context.Context, request api.CheckHealthRequestObject) (api.CheckHealthResponseObject, error) {
	return api.CheckHealth200TextResponse("OK"), nil
}

// Auth implements api.StrictServerInterface
func (a *Agent) Auth(ctx context.Context, request api.AuthRequestObject) (api.AuthResponseObject, error) {
	if request.Body == nil || request.Body.Token == "" {
		msg := "missing token"
		return api.Auth401JSONResponse{Success: false, Message: &msg}, nil
	}

	if subtle.ConstantTimeCompare([]byte(request.Body.Token), []byte(a.config.AuthToken)) != 1 {
		msg := "invalid token"
		return api.Auth401JSONResponse{Success: false, Message: &msg}, nil
	}

	// Note: Cookie setting is handled by middleware or manual handler if strictly required,
	// but StrictServerInterface doesn't easily support setting cookies on the response object
	// without a custom response type visit.
	// For now, we return success. The client might rely on the cookie which is a limitation here.
	// To fix this properly in strict mode, we'd need to access the ResponseWriter.
	// However, for the Agent, we can rely on the token being passed in headers for subsequent requests.
	return api.Auth200JSONResponse{Success: true}, nil
}

// Logout implements api.StrictServerInterface
func (a *Agent) Logout(ctx context.Context, request api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	// Cookie clearing would happen here if we had access to ResponseWriter
	return api.Logout200JSONResponse{Success: true}, nil
}

// CheckAuth implements api.StrictServerInterface
func (a *Agent) CheckAuth(ctx context.Context, request api.CheckAuthRequestObject) (api.CheckAuthResponseObject, error) {
	authenticated := true
	return api.CheckAuth200JSONResponse{Authenticated: &authenticated}, nil
}

// GetStatus implements api.StrictServerInterface
func (a *Agent) GetStatus(ctx context.Context, request api.GetStatusRequestObject) (api.GetStatusResponseObject, error) {
	return a.getStatusResponse()
}

// GetAgentStatus implements api.StrictServerInterface
func (a *Agent) GetAgentStatus(ctx context.Context, request api.GetAgentStatusRequestObject) (api.GetAgentStatusResponseObject, error) {
	// Agent status is the same as status for the agent
	resp, _ := a.getStatusResponse()
	return api.GetAgentStatus200JSONResponse(resp), nil
}

func (a *Agent) getStatusResponse() (api.GetStatus200JSONResponse, error) {
	hostname, _ := os.Hostname()
	uptime := time.Since(a.startTime).Round(time.Second).String()
	_, port, _ := net.SplitHostPort(a.config.ListenAddress)

	ip := getOutboundIP()
	var ipAddr api.StatusResponse_IpAddress
	if err := ipAddr.FromStatusResponseIpAddress0(ip); err != nil {
		return api.GetStatus200JSONResponse{}, err
	}

	// The agent is itself the best endpoint — a SPA already served from here
	// has nowhere better to hop to.
	endpoints := make([]string, 0, 1)
	if ip != "" && port != "" {
		endpoints = append(endpoints, fmt.Sprintf("http://%s:%s", ip, port))
	}

	return api.GetStatus200JSONResponse{
		Status:    "ok",
		Version:   "dev",
		Uptime:    uptime,
		Hostname:  hostname,
		IpAddress: ipAddr,
		Port:      port,
		Secret:    "",
		Endpoints: &endpoints,
	}, nil
}

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// GetLayouts implements api.StrictServerInterface
func (a *Agent) GetLayouts(ctx context.Context, request api.GetLayoutsRequestObject) (api.GetLayoutsResponseObject, error) {
	allLayouts := a.layouts.List()

	// Update current layout from display manager to ensure it's fresh. The
	// layout we last switched to breaks ties between layouts the display can't
	// tell apart, so it isn't replaced by an equally-matching sibling.
	if monitors, err := a.displayMgr.ListMonitors(); err == nil {
		if current, ok := a.layouts.GetClosest(monitors, a.currentLayout); ok {
			a.currentLayout = current
		}
	}

	// Sort by minimum integer alias (if any), then by ID
	sort.Slice(allLayouts, func(i, j int) bool {
		ai := minIntAlias(allLayouts[i].Aliases)
		aj := minIntAlias(allLayouts[j].Aliases)
		if ai != aj {
			// Layouts with an integer alias come first
			if ai == nil {
				return false
			}
			if aj == nil {
				return true
			}
			return *ai < *aj
		}
		return allLayouts[i].Id < allLayouts[j].Id
	})

	// allLayouts is already []api.Layout, no conversion needed
	return api.GetLayouts200JSONResponse{
		Layouts:       allLayouts,
		CurrentLayout: a.currentLayout,
	}, nil
}

// minIntAlias returns the smallest integer alias, or nil if none
func minIntAlias(aliases []string) *int {
	var result *int
	for _, a := range aliases {
		if v, err := strconv.Atoi(a); err == nil {
			if result == nil || v < *result {
				result = &v
			}
		}
	}
	return result
}

// SwitchLayout implements api.StrictServerInterface
func (a *Agent) SwitchLayout(ctx context.Context, request api.SwitchLayoutRequestObject) (api.SwitchLayoutResponseObject, error) {
	if request.Body == nil || request.Body.Layout == "" {
		return api.SwitchLayout400JSONResponse{Code: 400, Error: "layout name is required"}, nil
	}
	layoutName := request.Body.Layout

	log.Printf("Switching to layout: %s", layoutName)

	layouts := a.layouts.FindByIDOrAlias(layoutName)
	if len(layouts) == 0 {
		return api.SwitchLayout404JSONResponse{Code: 404, Error: fmt.Sprintf("layout %q not found", layoutName)}, nil
	}
	if len(layouts) > 1 {
		return api.SwitchLayout400JSONResponse{Code: 400, Error: fmt.Sprintf("layout %q is ambiguous", layoutName)}, nil
	}

	layout := layouts[0]

	// Prefer a backend that verifies the result: a display server accepting the
	// configuration is not proof it stuck, so reporting a bare success can be
	// misleading (it may be rolled back a second later, or have changed nothing).
	result := display.LayoutApplyResult{Outcome: display.OutcomeUnverified}
	var err error
	if v, ok := a.displayMgr.(display.VerifyingManager); ok {
		result, err = v.ApplyLayoutConfigVerified(layout)
	} else {
		err = a.displayMgr.ApplyLayoutConfig(layout)
	}
	if err != nil {
		log.Printf("Failed to apply layout: %v", err)
		return api.SwitchLayout500JSONResponse{Code: 500, Error: err.Error()}, nil
	}

	if result.Outcome.Ok() {
		log.Printf("Layout %q: %s (%s)", layoutName, result.Outcome, result.Detail)
	} else {
		// Loud, because this is the case that previously looked like success.
		log.Printf("WARNING: layout %q did not take effect: %s (%s)", layoutName, result.Outcome, result.Detail)
	}

	a.currentLayout = layoutName
	a.recordCurrentLayout(layout.Id)

	msg := result.Detail
	if msg == "" {
		msg = fmt.Sprintf("Switched to layout: %s", layoutName)
	}
	outcome := api.SwitchLayoutResponseOutcome(result.Outcome)
	return api.SwitchLayout200JSONResponse{
		Success:       true,
		CurrentLayout: layoutName,
		Message:       &msg,
		Outcome:       &outcome,
	}, nil
}

// GetCurrentLayout implements api.StrictServerInterface
func (a *Agent) GetCurrentLayout(ctx context.Context, request api.GetCurrentLayoutRequestObject) (api.GetCurrentLayoutResponseObject, error) {
	var currentLayout string
	monitors, err := a.displayMgr.ListMonitors()
	if err != nil {
		log.Printf("Failed to list monitors: %v", err)
		currentLayout = a.currentLayout
	} else {
		if layout, ok := a.layouts.GetClosest(monitors, a.currentLayout); ok {
			currentLayout = layout
		}
	}

	if currentLayout == "" {
		currentLayout = a.currentLayout
	}

	return api.GetCurrentLayout200JSONResponse{
		Success:       true,
		CurrentLayout: currentLayout,
	}, nil
}

// SaveCurrentLayout implements api.StrictServerInterface
// captureCurrentMonitors snapshots the currently active display configuration
// as layout monitors. Inactive (disabled) monitors are omitted, so an empty
// result means nothing is currently driving a display.
func (a *Agent) captureCurrentMonitors() ([]api.LayoutMonitor, error) {
	monitors, err := a.displayMgr.ListMonitors()
	if err != nil {
		return nil, err
	}
	configs := make([]api.LayoutMonitor, 0, len(monitors))
	for _, m := range monitors {
		if m.Active == nil {
			continue
		}
		configs = append(configs, api.LayoutMonitor{
			Name:        m.Name,
			Edid:        m.Edid,
			Port:        m.Port,
			Width:       m.Active.Width,
			Height:      m.Active.Height,
			RefreshRate: m.Active.RefreshRate,
			PositionX:   m.Active.PositionX,
			PositionY:   m.Active.PositionY,
			Primary:     m.Active.Primary,
			Scale:       m.Active.Scale,
		})
	}
	return configs, nil
}

func (a *Agent) SaveCurrentLayout(ctx context.Context, request api.SaveCurrentLayoutRequestObject) (api.SaveCurrentLayoutResponseObject, error) {
	if request.Body == nil || request.Body.Name == "" {
		return api.SaveCurrentLayout400JSONResponse{Code: 400, Error: "name is required"}, nil
	}
	name := request.Body.Name
	id := ""
	if request.Body.Id != nil {
		id = *request.Body.Id
	}

	// Generate ID if not provided
	if id == "" {
		id = slugify(name)
	}

	monitorConfigs, err := a.captureCurrentMonitors()
	if err != nil {
		return api.SaveCurrentLayout500JSONResponse{Code: 500, Error: "failed to get current monitors"}, nil
	}

	layout := api.Layout{
		Id:       id,
		Name:     name,
		Emoji:    request.Body.Emoji,
		Aliases:  []string{},
		Monitors: monitorConfigs,
	}

	a.layouts.Set(layout)

	// Save to config file
	if err := a.saveLayouts(); err != nil {
		log.Printf("Failed to save layouts: %v", err)
		return api.SaveCurrentLayout500JSONResponse{Code: 500, Error: "failed to save layout"}, nil
	}

	return api.SaveCurrentLayout200JSONResponse{
		Success: true,
		Layout:  layout,
	}, nil
}

// RemoveLayout implements api.StrictServerInterface
func (a *Agent) RemoveLayout(ctx context.Context, request api.RemoveLayoutRequestObject) (api.RemoveLayoutResponseObject, error) {
	if request.Body == nil || request.Body.Layout == "" {
		return api.RemoveLayout400JSONResponse{Code: 400, Error: "layout name is required"}, nil
	}
	layoutName := request.Body.Layout

	if _, ok := a.layouts.Get(layoutName); !ok {
		return api.RemoveLayout404JSONResponse{Code: 404, Error: fmt.Sprintf("layout %q not found", layoutName)}, nil
	}

	log.Printf("Removing layout: %s", layoutName)
	a.layouts.Delete(layoutName)

	if a.currentLayout == layoutName {
		a.currentLayout = ""
	}

	if err := a.saveLayouts(); err != nil {
		log.Printf("Failed to save layouts: %v", err)
		return api.RemoveLayout500JSONResponse{Code: 500, Error: "failed to save config"}, nil
	}

	msg := fmt.Sprintf("Removed layout: %s", layoutName)
	return api.RemoveLayout200JSONResponse{
		Success: true,
		Message: &msg,
	}, nil
}

// UpdateLayout implements api.StrictServerInterface
func (a *Agent) UpdateLayout(ctx context.Context, request api.UpdateLayoutRequestObject) (api.UpdateLayoutResponseObject, error) {
	if request.Body == nil || request.Body.Id == "" {
		return api.UpdateLayout400JSONResponse{Code: 400, Error: "layout id is required"}, nil
	}
	id := request.Body.Id

	if _, ok := a.layouts.Get(id); !ok {
		return api.UpdateLayout404JSONResponse{Code: 404, Error: fmt.Sprintf("layout %q not found", id)}, nil
	}

	// Normalise and validate the new alias set: trim, drop blanks/dupes, and
	// reject any alias already claimed by another layout (or matching another
	// layout's ID), which would make switching ambiguous.
	if request.Body.Aliases != nil {
		seen := make(map[string]bool)
		cleaned := make([]string, 0, len(*request.Body.Aliases))
		for _, raw := range *request.Body.Aliases {
			alias := strings.TrimSpace(raw)
			if alias == "" || seen[alias] {
				continue
			}
			if alias == id {
				continue // an alias equal to the layout's own ID is redundant
			}
			if owner := a.layouts.AliasOwner(alias, id); owner != "" {
				return api.UpdateLayout400JSONResponse{Code: 400, Error: fmt.Sprintf("alias %q is already used by layout %q", alias, owner)}, nil
			}
			seen[alias] = true
			cleaned = append(cleaned, alias)
		}
		request.Body.Aliases = &cleaned
	}

	if request.Body.Name != nil {
		trimmed := strings.TrimSpace(*request.Body.Name)
		if trimmed == "" {
			return api.UpdateLayout400JSONResponse{Code: 400, Error: "name cannot be empty"}, nil
		}
		request.Body.Name = &trimmed
	}

	// Optionally re-capture the layout's geometry from the live display, so an
	// existing layout can be brought up to date in place after adjusting the
	// monitors — rather than having to delete it and save a new one, which would
	// lose its id, aliases and position in the list.
	captured := false
	if request.Body.CaptureMonitors != nil && *request.Body.CaptureMonitors {
		monitors, err := a.captureCurrentMonitors()
		if err != nil {
			log.Printf("Failed to read monitors while re-capturing layout %q: %v", id, err)
			return api.UpdateLayout500JSONResponse{Code: 500, Error: "failed to read the current display configuration"}, nil
		}
		if len(monitors) == 0 {
			return api.UpdateLayout400JSONResponse{Code: 400, Error: "no active monitors to capture"}, nil
		}
		a.layouts.SetMonitors(id, monitors)
		captured = true
	}

	layout, _ := a.layouts.UpdateMeta(id, request.Body.Name, request.Body.Emoji, request.Body.Aliases)

	if err := a.saveLayouts(); err != nil {
		log.Printf("Failed to save layouts: %v", err)
		return api.UpdateLayout500JSONResponse{Code: 500, Error: "failed to save layout"}, nil
	}

	if captured {
		log.Printf("Updated layout: %s (re-captured %d monitors from the current display)", id, len(layout.Monitors))
	} else {
		log.Printf("Updated layout: %s", id)
	}
	return api.UpdateLayout200JSONResponse{
		Success: true,
		Layout:  &layout,
	}, nil
}

// GetMonitors implements api.StrictServerInterface
func (a *Agent) GetMonitors(ctx context.Context, request api.GetMonitorsRequestObject) (api.GetMonitorsResponseObject, error) {
	monitors, err := a.displayMgr.ListMonitors()
	if err != nil {
		return api.GetMonitors502JSONResponse{Code: 502, Error: err.Error()}, nil
	}

	apiMonitors := make([]api.Monitor, 0)
	for _, m := range monitors {
		mon := api.Monitor{
			Edid:         m.Edid,
			Manufacturer: m.Manufacturer,
			Name:         m.Name,
			Port:         m.Port,
		}
		if m.Active != nil {
			mon.Active = &api.ActiveMonitor{
				Height:      m.Active.Height,
				Model:       m.Active.Model,
				PositionX:   m.Active.PositionX,
				PositionY:   m.Active.PositionY,
				Primary:     m.Active.Primary,
				RefreshRate: m.Active.RefreshRate,
				Width:       m.Active.Width,
				Scale:       m.Active.Scale,
			}
		}
		apiMonitors = append(apiMonitors, mon)
	}

	// Some monitors should stay visible even when they aren't currently
	// enumerated as connected displays — a configured TV drops off HDMI when
	// powered off, and a monitor that's part of a saved layout is one the user
	// cares about and may just have turned off. Inject synthetic (inactive)
	// entries for those so their cards persist; enrich() then tags each with its
	// backend + capabilities (a disconnected DDC monitor gets no live controls).
	present := make(map[string]bool, len(apiMonitors))
	for _, m := range apiMonitors {
		present[m.Edid] = true
	}
	remember := func(edid, name string) {
		if edid == "" || present[edid] {
			return
		}
		present[edid] = true
		apiMonitors = append(apiMonitors, api.Monitor{Edid: edid, Name: name})
	}

	// The configured TVs (still controllable over the network when off).
	for _, tvEntry := range a.control.registry.TVEntries() {
		name := tvEntry.FriendlyName
		if name == "" {
			name = "TV"
		}
		remember(tvEntry.Edid, name)
	}
	// Every monitor referenced by a saved layout.
	for _, layout := range a.layouts.List() {
		for _, lm := range layout.Monitors {
			remember(lm.Edid, lm.Name)
		}
	}

	// Add registry + control metadata (friendly name, backend, capabilities,
	// current brightness, visibility) so any frontend renders the right controls.
	apiMonitors = a.control.enrich(apiMonitors)

	return api.GetMonitors200JSONResponse(apiMonitors), nil
}

// Shutdown implements api.StrictServerInterface
func (a *Agent) Shutdown(ctx context.Context, request api.ShutdownRequestObject) (api.ShutdownResponseObject, error) {
	log.Println("Shutdown requested via API")

	// Flush response, then shut down after a brief delay
	go func() {
		time.Sleep(1 * time.Second)

		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("shutdown", "/s", "/t", "1")
		case "linux":
			cmd = exec.Command("systemctl", "poweroff")
		default:
			log.Printf("Shutdown not supported on %s", runtime.GOOS)
			return
		}

		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Shutdown command failed: %v: %s", err, string(out))
		}
	}()

	msg := "Shutdown initiated"
	return api.Shutdown200JSONResponse{
		Success: true,
		Message: &msg,
	}, nil
}

// ConnectTrackpad implements api.StrictServerInterface (stub, handled by wrapper)
func (a *Agent) ConnectTrackpad(ctx context.Context, request api.ConnectTrackpadRequestObject) (api.ConnectTrackpadResponseObject, error) {
	return nil, nil
}

// Wake implements api.StrictServerInterface (stub)
func (a *Agent) Wake(ctx context.Context, request api.WakeRequestObject) (api.WakeResponseObject, error) {
	return api.Wake404JSONResponse{Code: 404, Error: "not supported on agent"}, nil
}

// SimReset implements api.StrictServerInterface (stub)
func (a *Agent) SimReset(ctx context.Context, request api.SimResetRequestObject) (api.SimResetResponseObject, error) {
	return api.SimReset404JSONResponse{Code: 404, Error: "not supported on agent"}, nil
}

// SimSetState implements api.StrictServerInterface (stub)
func (a *Agent) SimSetState(ctx context.Context, request api.SimSetStateRequestObject) (api.SimSetStateResponseObject, error) {
	return api.SimSetState404JSONResponse{Code: 404, Error: "not supported on agent"}, nil
}

// SimState implements api.StrictServerInterface (stub)
func (a *Agent) SimState(ctx context.Context, request api.SimStateRequestObject) (api.SimStateResponseObject, error) {
	return api.SimState404JSONResponse{Code: 404, Error: "not supported on agent"}, nil
}

// handleTrackpad handles WebSocket connections for trackpad input
// This is called by the wrapper, bypassing the strict handler
func (a *Agent) handleTrackpad(w http.ResponseWriter, r *http.Request) {
	// Input control is unavailable in greeter mode (no uinput as the gdm user).
	if a.mouse == nil || a.keyboard == nil {
		http.Error(w, "input control not available", http.StatusServiceUnavailable)
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Printf("Trackpad WebSocket accept error: %v", err)
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	modifierToKey := func(m api.Modifier) string {
		switch m {
		case api.Shift:
			return "Shift"
		case api.Ctrl:
			return "Control"
		case api.Alt:
			return "Alt"
		case api.Meta:
			return "Meta"
		}
		return ""
	}

	currentModifiers := make(map[string]bool)
	updateModifiers := func(mods []api.Modifier) {
		target := make(map[string]bool)
		for _, m := range mods {
			if k := modifierToKey(m); k != "" {
				target[k] = true
			}
		}
		for k := range currentModifiers {
			if !target[k] {
				a.keyboard.KeyUp(k, nil)
				delete(currentModifiers, k)
			}
		}
		for k := range target {
			if !currentModifiers[k] {
				a.keyboard.KeyDown(k, nil)
				currentModifiers[k] = true
			}
		}
	}

	var latestX, latestY atomic.Int32
	var posReady atomic.Bool

	// Send an initial message immediately so the frontend detects the
	// connection: it treats the first message as proof the agent end of the
	// proxy is live. Prefer the real cursor position (X11), but fall back to a
	// bare "connected" ping when position is unreadable — e.g. the uinput
	// backend on Wayland, where the socket and input injection work fine but
	// the cursor position can't be queried.
	msg := api.TrackpadMessage{}
	mx, my, posErr := a.mouse.GetPosition()
	posSupported := posErr == nil
	if posSupported {
		msg.FromTrackpadMessageMousePositionUpdate(api.TrackpadMessageMousePositionUpdate{X: mx, Y: my})
		latestX.Store(int32(mx))
		latestY.Store(int32(my))
	} else {
		msg.FromTrackpadMessageConnected(api.TrackpadMessageConnected{Type: api.Connected})
	}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return
	}

	// Position update sender goroutine (60Hz), skip if position unchanged. Only
	// runs when the backend can read the cursor position; otherwise it would
	// busy-poll a call that always errors and never send anything.
	var lastSentX, lastSentY atomic.Int32
	lastSentX.Store(latestX.Load())
	lastSentY.Store(latestY.Load())
	if posSupported {
		go func() {
			ticker := time.NewTicker(16 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					var x, y int32
					if !posReady.Swap(false) {
						// Poll current position to catch external movement
						mx, my, err := a.mouse.GetPosition()
						if err != nil {
							continue
						}
						x, y = int32(mx), int32(my)
					} else {
						x = latestX.Load()
						y = latestY.Load()
					}
					if x == lastSentX.Load() && y == lastSentY.Load() {
						continue
					}
					lastSentX.Store(x)
					lastSentY.Store(y)
					msg := api.TrackpadMessage{}
					msg.FromTrackpadMessageMousePositionUpdate(api.TrackpadMessageMousePositionUpdate{
						X: int(x),
						Y: int(y),
					})
					data, _ := json.Marshal(msg)
					if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
						return
					}
				}
			}
		}()
	}

	// Track what the client is currently holding down so we can release it if the
	// connection drops or a key-up never arrives (mobile keyboards are unreliable
	// about firing key-up). Without this, a held key stays pressed on the virtual
	// device and wedges input until the user logs out.
	heldKeys := make(map[string]bool)
	heldButtons := make(map[input.MouseButton]bool)
	defer func() {
		for key := range heldKeys {
			a.keyboard.KeyUp(key, nil)
		}
		for btn := range heldButtons {
			a.mouse.ButtonUp(btn)
		}
		updateModifiers(nil) // release any held modifier keys
	}()

	// Read loop
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			break
		}

		var msg api.TrackpadMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		castMsg, err := msg.ValueByDiscriminator()
		if err != nil {
			continue
		}

		switch v := castMsg.(type) {
		case api.TrackpadMessageMouseMoveRelative:
			a.mouse.MoveRelative(v.Dx, v.Dy)
		case api.TrackpadMessageMouseClick:
			updateModifiers(v.Modifiers)
			a.mouse.Click(input.ParseMouseButton(string(v.Btn)))
		case api.TrackpadMessageMouseDown:
			updateModifiers(v.Modifiers)
			btn := input.ParseMouseButton(string(v.Btn))
			a.mouse.ButtonDown(btn)
			heldButtons[btn] = true
		case api.TrackpadMessageMouseUp:
			updateModifiers(v.Modifiers)
			btn := input.ParseMouseButton(string(v.Btn))
			a.mouse.ButtonUp(btn)
			delete(heldButtons, btn)
		case api.TrackpadMessageMouseScroll:
			precise := v.Precise != nil && *v.Precise
			a.mouse.Scroll(int(v.Dx), int(v.Dy), precise)
		case api.TrackpadMessageKeyDown:
			updateModifiers(v.Modifiers)
			a.keyboard.KeyDown(v.Key, nil)
			heldKeys[v.Key] = true
		case api.TrackpadMessageKeyUp:
			updateModifiers(v.Modifiers)
			a.keyboard.KeyUp(v.Key, nil)
			delete(heldKeys, v.Key)
		case api.TrackpadMessageText:
			a.typeText(v.Text)
		case api.TrackpadMessageMouseMoveTo:
			a.mouse.MoveTo(v.X, v.Y)
		}
	}
}

// typeText types a run of text (from a mobile keyboard's input event, where the
// individual keydown/keyup events are unreliable) as a sequence of discrete
// key presses. Each character is pressed and released immediately so nothing is
// left held. Shift is applied for characters that need it on a US layout.
func (a *Agent) typeText(text string) {
	for _, r := range text {
		var mods []string
		if charNeedsShift(r) {
			mods = []string{"shift"}
		}
		key := string(r)
		a.keyboard.KeyDown(key, mods)
		a.keyboard.KeyUp(key, mods)
	}
}

// charNeedsShift reports whether typing r on a US keyboard layout requires the
// Shift modifier. The backends map a character and its shifted glyph to the same
// physical key, so Shift must be supplied out of band.
func charNeedsShift(r rune) bool {
	if r >= 'A' && r <= 'Z' {
		return true
	}
	switch r {
	case '!', '@', '#', '$', '%', '^', '&', '*', '(', ')',
		'_', '+', '{', '}', '|', ':', '"', '~', '<', '>', '?':
		return true
	}
	return false
}

// Run starts the agent
func Run(config *config.AgentConfig) error {
	agent, err := New(config)
	if err != nil {
		return err
	}

	return agent.Start()
}

// RunGreeter starts the agent in GDM greeter mode (display/layouts only).
func RunGreeter(config *config.AgentConfig) error {
	agent, err := NewGreeter(config)
	if err != nil {
		return err
	}

	return agent.Start()
}

type logResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *logResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Start starts the HTTP server
func (a *Agent) Start() error {
	a.server = &http.Server{
		Addr:         a.config.ListenAddress,
		Handler:      common.LoggingMiddleware(common.HealthCORS(a.router)),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Agent starting at http://%s", a.config.ListenAddress)
		ln, err := common.ListenWithRetry("tcp", a.config.ListenAddress)
		if err != nil {
			log.Fatalf("Server error: %v", err)
		}
		if err := a.server.Serve(ln); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down agent...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return a.server.Shutdown(ctx)
}

// CheckStatus checks if an agent is reachable
func CheckStatus(addr string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/health", addr))
	if err != nil {
		return fmt.Sprintf("ERROR: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return "OK"
	}
	return fmt.Sprintf("ERROR: status %d", resp.StatusCode)
}

// slugify converts a string into a URL-friendly slug
func slugify(input string) string {
	// Replace spaces with dashes and remove non-alphanumeric characters
	slug := regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(strings.ToLower(input), "-")
	return strings.Trim(slug, "-")
}

// saveLayouts persists the current layouts to the data-dir store. The config
// file is deliberately left untouched so that redeploying the config template
// can never clobber saved layouts.
func (a *Agent) saveLayouts() error {
	if err := a.layoutStore.Save(a.layouts.ToSlice()); err != nil {
		return err
	}
	a.mirrorToGreeter()
	return nil
}

// recordCurrentLayout persists the last-applied layout ID so the greeter agent
// can restore it on the next login screen. The greeter itself doesn't record —
// it follows the user's session choice rather than setting it.
func (a *Agent) recordCurrentLayout(id string) {
	if a.greeter {
		return
	}
	if err := store.SaveCurrentLayout(id); err != nil {
		log.Printf("Warning: failed to record current layout: %v", err)
	}
	a.mirrorToGreeter()
}

// greeterDataDir is the gdm-readable copy the login-screen agent reads its
// layouts + current-layout from (see internal/agent/hostsetup.go greeterRoot).
const greeterDataDir = "/var/lib/ottoman/greeter/.local/share/ottoman"

// mirrorToGreeter keeps the greeter's copy of layouts + current-layout in sync
// with the user's, so the login screen reflects the latest state. No-op unless
// the greeter agent is installed. Best-effort; failures are logged, not fatal.
func (a *Agent) mirrorToGreeter() {
	if a.greeter {
		return
	}
	if _, err := os.Stat(greeterDataDir); err != nil {
		// Only absence means "not installed" — anything else (e.g. a
		// permission-denied traversal) would silently strand the greeter on
		// stale layouts, so say so.
		if !os.IsNotExist(err) {
			log.Printf("Warning: cannot access greeter data dir: %v", err)
		}
		return
	}
	for _, name := range []string{"layouts.json", "current-layout"} {
		if err := copyFileInto(filepath.Join(store.DataDir(), name), filepath.Join(greeterDataDir, name)); err != nil {
			log.Printf("Warning: failed to mirror %s to greeter: %v", name, err)
		}
	}
}

// copyFileInto copies src to dst atomically (temp + rename in dst's dir, so the
// group/setgid inheritance of that dir applies). A missing src is not an error.
func copyFileInto(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0640); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
