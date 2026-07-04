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
	tv            *tvController
	displayMgr    display.Manager
	mouse         input.MouseController
	keyboard      input.KeyboardController
	audio         audio.Controller
	startTime     time.Time
	currentLayout string
}

// Ensure Agent implements StrictServerInterface
var _ api.StrictServerInterface = (*Agent)(nil)

// New creates a new agent instance
func New(cfg *config.AgentConfig) (*Agent, error) {
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

	// Create mouse controller
	mouse, err := input.NewMouseController()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create mouse controller")
	}

	// Create keyboard controller
	keyboard, err := input.NewKeyboardController()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create keyboard controller")
	}

	// Audio control is optional: if PipeWire/wpctl isn't available the agent
	// still runs, and the audio endpoints report the feature as unavailable.
	audioCtl, err := audio.NewController()
	if err != nil {
		log.Printf("Audio control unavailable: %v", err)
		audioCtl = nil
	}

	// Monitor registry (friendly names, control backends, visibility) lives in
	// the data dir alongside layouts.
	registry, err := store.NewRegistry("")
	if err != nil {
		return nil, errors.Wrap(err, "failed to load monitor registry")
	}

	// TV controller (LG webOS). Its transport is resolved from the monitor
	// registry (the "tv"-backend entry); cfg.TV is a legacy fallback. The
	// pairing key lives in the data dir, not the config, so a config redeploy
	// can't drop it.
	tv := newTVController(registry, cfg.TV, store.NewTVStore(""))
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
	}

	if err := a.setupRoutes(); err != nil {
		return nil, err
	}

	return a, nil
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

	var ipAddr api.StatusResponse_IpAddress
	if err := ipAddr.FromStatusResponseIpAddress0(getOutboundIP()); err != nil {
		return api.GetStatus200JSONResponse{}, err
	}

	return api.GetStatus200JSONResponse{
		Status:    "ok",
		Version:   "dev",
		Uptime:    uptime,
		Hostname:  hostname,
		IpAddress: ipAddr,
		Port:      port,
		Secret:    "",
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

	// Update current layout from display manager to ensure it's fresh
	if monitors, err := a.displayMgr.ListMonitors(); err == nil {
		if current, ok := a.layouts.GetClosest(monitors); ok {
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

	if err := a.displayMgr.ApplyLayoutConfig(layout); err != nil {
		log.Printf("Failed to apply layout: %v", err)
		return api.SwitchLayout500JSONResponse{Code: 500, Error: err.Error()}, nil
	}

	a.currentLayout = layoutName

	msg := fmt.Sprintf("Switched to layout: %s", layoutName)
	return api.SwitchLayout200JSONResponse{
		Success:       true,
		CurrentLayout: layoutName,
		Message:       &msg,
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
		if layout, ok := a.layouts.GetClosest(monitors); ok {
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

	// Get current monitor state to save
	monitors, err := a.displayMgr.ListMonitors()
	if err != nil {
		return api.SaveCurrentLayout500JSONResponse{Code: 500, Error: "failed to get current monitors"}, nil
	}

	// Convert Monitor to LayoutMonitor config
	monitorConfigs := make([]api.LayoutMonitor, 0)
	for _, m := range monitors {
		if m.Active != nil {
			monitorConfigs = append(monitorConfigs, api.LayoutMonitor{
				Name:        m.Name,
				Edid:        m.Edid,
				Port:        m.Port,
				Width:       m.Active.Width,
				Height:      m.Active.Height,
				RefreshRate: m.Active.RefreshRate,
				PositionX:   m.Active.PositionX,
				PositionY:   m.Active.PositionY,
				Primary:     m.Active.Primary,
			})
		}
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

	// The configured TV (still controllable over the network when off).
	if tvEntry, ok := a.control.registry.TVEntry(); ok {
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

	// Send initial position immediately so the frontend detects connection
	if mx, my, err := a.mouse.GetPosition(); err == nil {
		msg := api.TrackpadMessage{}
		msg.FromTrackpadMessageMousePositionUpdate(api.TrackpadMessageMousePositionUpdate{
			X: mx,
			Y: my,
		})
		data, _ := json.Marshal(msg)
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			return
		}
		latestX.Store(int32(mx))
		latestY.Store(int32(my))
	}

	// Position update sender goroutine (60Hz), skip if position unchanged
	var lastSentX, lastSentY atomic.Int32
	lastSentX.Store(latestX.Load())
	lastSentY.Store(latestY.Load())
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
			a.mouse.ButtonDown(input.ParseMouseButton(string(v.Btn)))
		case api.TrackpadMessageMouseUp:
			updateModifiers(v.Modifiers)
			a.mouse.ButtonUp(input.ParseMouseButton(string(v.Btn)))
		case api.TrackpadMessageMouseScroll:
			precise := v.Precise != nil && *v.Precise
			a.mouse.Scroll(int(v.Dx), int(v.Dy), precise)
		case api.TrackpadMessageKeyDown:
			updateModifiers(v.Modifiers)
			a.keyboard.KeyDown(v.Key, nil)
		case api.TrackpadMessageKeyUp:
			updateModifiers(v.Modifiers)
			a.keyboard.KeyUp(v.Key, nil)
		case api.TrackpadMessageMouseMoveTo:
			a.mouse.MoveTo(v.X, v.Y)
		}
	}
}

// Run starts the agent
func Run(config *config.AgentConfig) error {
	agent, err := New(config)
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
		Handler:      common.LoggingMiddleware(a.router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Agent starting at http://%s", a.config.ListenAddress)
		if err := a.server.ListenAndServe(); err != http.ErrServerClosed {
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
	return a.layoutStore.Save(a.layouts.ToSlice())
}
