package controller

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/common"
	"github.com/trolleyman/ottoman/internal/config"
	"github.com/trolleyman/ottoman/internal/display"
	"github.com/trolleyman/ottoman/internal/input"
)

type agentState int

const (
	agentOffline agentState = iota
	agentBooting
	agentOnline
)

func (s agentState) String() string {
	switch s {
	case agentOffline:
		return "offline"
	case agentBooting:
		return "booting"
	case agentOnline:
		return "online"
	default:
		return "unknown"
	}
}

// SimulatedController serves the real frontend with mocked API endpoints for testing WoL flows.
type SimulatedController struct {
	controllerCfg *config.ControllerConfig
	router        *http.ServeMux
	server        *http.Server
	startTime     time.Time

	mu        sync.RWMutex
	state     agentState
	bootTimer *time.Timer
	bootDelay time.Duration

	layouts         *display.Layouts
	currentLayout   string
	monitors        []api.Monitor
	trackpadCancels []context.CancelFunc
}

// RunSimulatedController creates and starts a simulated controller.
func RunSimulatedController(controllerCfg *config.ControllerConfig, agentCfg *config.AgentConfig, bootDelay time.Duration, startOnline bool) error {
	sim, err := NewSimulatedController(controllerCfg, agentCfg, bootDelay, startOnline)
	if err != nil {
		return err
	}
	return sim.Start()
}

// NewSimulatedController creates a new simulated controller instance.
func NewSimulatedController(controllerCfg *config.ControllerConfig, agentCfg *config.AgentConfig, bootDelay time.Duration, startOnline bool) (*SimulatedController, error) {
	if controllerCfg.AuthToken == "" {
		return nil, errors.New("controller config requires auth_token")
	}

	s := &SimulatedController{
		controllerCfg: controllerCfg,
		bootDelay:     bootDelay,
		startTime:     time.Now(),
	}

	if startOnline {
		s.state = agentOnline
	}

	// Load layouts from agent config
	s.layouts = display.NewLayoutsFromSlice(agentCfg.Layouts)

	// Sort layouts to pick default
	sorted := make([]api.Layout, len(agentCfg.Layouts))
	copy(sorted, agentCfg.Layouts)
	display.SortLayouts(sorted)

	// Set initial current layout to first layout
	if len(sorted) > 0 {
		s.currentLayout = sorted[0].Id
	}

	// Derive monitors from layouts
	s.monitors = deriveMonitors(agentCfg.Layouts)

	// Update monitor active states to match current layout
	s.updateMonitorStates()

	if err := s.setupRoutes(); err != nil {
		return nil, err
	}

	return s, nil
}

// deriveMonitors collects unique monitors by EDID across all layouts.
func deriveMonitors(layouts []api.Layout) []api.Monitor {
	seen := make(map[string]bool)
	var monitors []api.Monitor

	for _, layout := range layouts {
		for _, m := range layout.Monitors {
			if m.Edid == "" || seen[m.Edid] {
				continue
			}
			seen[m.Edid] = true

			// Extract manufacturer from EDID (format "MFR:PRODUCT")
			manufacturer := m.Edid
			if idx := strings.Index(m.Edid, ":"); idx >= 0 {
				manufacturer = m.Edid[:idx]
			}

			monitors = append(monitors, api.Monitor{
				Edid:         m.Edid,
				Port:         m.Port,
				Name:         m.Name,
				Manufacturer: manufacturer,
			})
		}
	}
	return monitors
}

// updateMonitorStates sets Active on each monitor based on the current layout.
func (s *SimulatedController) updateMonitorStates() {
	layout, ok := s.layouts.Get(s.currentLayout)
	if !ok {
		// No current layout — no monitors connected
		s.monitors = nil
		return
	}

	// Rebuild monitors list from the current layout only
	s.monitors = make([]api.Monitor, 0, len(layout.Monitors))
	for _, lm := range layout.Monitors {
		// Extract manufacturer from EDID
		manufacturer := lm.Edid
		if idx := strings.Index(lm.Edid, ":"); idx >= 0 {
			manufacturer = lm.Edid[:idx]
		}

		mon := api.Monitor{
			Edid:         lm.Edid,
			Port:         lm.Port,
			Name:         lm.Name,
			Manufacturer: manufacturer,
		}

		// Only mark as active if dimensions are non-zero
		if lm.Width > 0 && lm.Height > 0 {
			mon.Active = &api.ActiveMonitor{
				Width:       lm.Width,
				Height:      lm.Height,
				RefreshRate: lm.RefreshRate,
				PositionX:   lm.PositionX,
				PositionY:   lm.PositionY,
				Primary:     lm.Primary,
				Model:       lm.Name,
			}
		}
		s.monitors = append(s.monitors, mon)
	}
}

func (s *SimulatedController) setupRoutes() error {
	// Create a wrapper mux that intercepts the trackpad endpoint
	innerMux := http.NewServeMux()

	// Create the strict handler
	strictHandler := api.NewStrictHandler(s, nil)

	// Register all handlers on inner mux
	api.HandlerWithOptions(strictHandler, api.StdHTTPServerOptions{
		BaseRouter: innerMux,
	})

	if err := common.SetupSPAHandler(innerMux); err != nil {
		return errors.Wrap(err, "failed to setup SPA handler")
	}

	// Create outer mux that intercepts trackpad and delegates rest to inner
	s.router = http.NewServeMux()
	s.router.HandleFunc("GET /api/trackpad", s.handleTrackpadWebSocket)
	s.router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Skip trackpad endpoint, delegate everything else to inner mux
		if r.Method == "GET" && r.URL.Path == "/api/trackpad" {
			http.NotFound(w, r)
			return
		}
		innerMux.ServeHTTP(w, r)
	})

	return nil
}

// --- Standard handlers (identical to real server) ---

func (s *SimulatedController) CheckHealth(ctx context.Context, request api.CheckHealthRequestObject) (api.CheckHealthResponseObject, error) {
	return api.CheckHealth200TextResponse("OK"), nil
}

func (s *SimulatedController) GetStatus(ctx context.Context, request api.GetStatusRequestObject) (api.GetStatusResponseObject, error) {
	_, port, _ := net.SplitHostPort(s.controllerCfg.ListenAddress)
	if port == "" {
		port = "80"
	}

	uptime := time.Since(s.startTime).Round(time.Second).String()

	var ipAddr api.StatusResponse_IpAddress
	if err := ipAddr.FromStatusResponseIpAddress0(s.controllerCfg.Agent.IPAddress); err != nil {
		return nil, err
	}

	return api.GetStatus200JSONResponse{
		Status:    "ok",
		Version:   "dev",
		Uptime:    uptime,
		Hostname:  "",
		IpAddress: ipAddr,
		Port:      port,
		Secret:    "simulated-secret",
	}, nil
}

// GetAgentStatus returns detailed status of the simulated agent
func (s *SimulatedController) GetAgentStatus(ctx context.Context, request api.GetAgentStatusRequestObject) (api.GetAgentStatusResponseObject, error) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	if state != agentOnline {
		return api.GetAgentStatus502JSONResponse{
			Code:  http.StatusBadGateway,
			Error: "Bad Gateway (Client unreachable)",
		}, nil
	}

	hostname, _ := os.Hostname()
	uptime := time.Since(s.startTime).Round(time.Second).String()

	var ipAddr api.StatusResponse_IpAddress
	if err := ipAddr.FromStatusResponseIpAddress0(getOutboundIP()); err != nil {
		return nil, err
	}

	return api.GetAgentStatus200JSONResponse{
		Status:    "ok",
		Version:   "dev",
		Uptime:    uptime,
		Hostname:  hostname,
		IpAddress: ipAddr,
		Port:      fmt.Sprintf("%d", s.controllerCfg.Agent.Port),
		Secret:    "",
	}, nil
}

func (s *SimulatedController) Auth(ctx context.Context, request api.AuthRequestObject) (api.AuthResponseObject, error) {
	if request.Body == nil || request.Body.Token == "" {
		msg := "missing token"
		return api.Auth401JSONResponse{Success: false, Message: &msg}, nil
	}

	if subtle.ConstantTimeCompare([]byte(request.Body.Token), []byte(s.controllerCfg.AuthToken)) != 1 {
		msg := "invalid token"
		return api.Auth401JSONResponse{Success: false, Message: &msg}, nil
	}

	// Note: Cookie setting is not supported in strict mode without custom response handling.
	// In a real implementation, we'd use a middleware or custom handler.
	return api.Auth200JSONResponse{Success: true}, nil
}

func (s *SimulatedController) Logout(ctx context.Context, request api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	return api.Logout200JSONResponse{Success: true}, nil
}

func (s *SimulatedController) CheckAuth(ctx context.Context, request api.CheckAuthRequestObject) (api.CheckAuthResponseObject, error) {
	authenticated := true
	return api.CheckAuth200JSONResponse{Authenticated: &authenticated}, nil
}

// --- Simulated WoL handlers ---

func (s *SimulatedController) Wake(ctx context.Context, request api.WakeRequestObject) (api.WakeResponseObject, error) {
	s.mu.Lock()
	macAddr := s.controllerCfg.Agent.MACAddress
	state := s.state

	if macAddr == "" {
		s.mu.Unlock()
		return api.Wake404JSONResponse{
			Code:  http.StatusNotFound,
			Error: "no wake target configured",
		}, nil
	}

	switch state {
	case agentOffline:
		s.state = agentBooting
		s.bootTimer = time.AfterFunc(s.bootDelay, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.state == agentBooting {
				s.state = agentOnline
				log.Printf("[SIM] Agent is now ONLINE (boot complete after %s)", s.bootDelay)
			}
		})
		s.mu.Unlock()
		log.Printf("[SIM] WoL sent to %s — client will be online in %s", macAddr, s.bootDelay)
		msg := fmt.Sprintf("Wake-on-LAN packet sent to %s", macAddr)
		return api.Wake200JSONResponse{
			Success: true,
			Message: &msg,
		}, nil

	case agentBooting:
		s.mu.Unlock()
		log.Printf("[SIM] WoL sent to %s — already booting", macAddr)
		msg := fmt.Sprintf("Wake-on-LAN packet sent to %s (already booting)", macAddr)
		return api.Wake200JSONResponse{
			Success: true,
			Message: &msg,
		}, nil

	case agentOnline:
		s.mu.Unlock()
		log.Printf("[SIM] WoL sent to %s — already online", macAddr)
		msg := fmt.Sprintf("Wake-on-LAN packet sent to %s (already online)", macAddr)
		return api.Wake200JSONResponse{
			Success: true,
			Message: &msg,
		}, nil
	}
	return api.Wake500JSONResponse{
		Code:  http.StatusInternalServerError,
		Error: "unknown state",
	}, nil
}

// --- Simulated display handlers ---

func (s *SimulatedController) Shutdown(ctx context.Context, request api.ShutdownRequestObject) (api.ShutdownResponseObject, error) {
	s.mu.Lock()
	if s.bootTimer != nil {
		s.bootTimer.Stop()
		s.bootTimer = nil
	}
	s.state = agentOffline
	s.cancelTrackpads()
	s.mu.Unlock()

	log.Printf("[SIM] Agent shut down via API — now OFFLINE")
	msg := "Shutdown initiated"
	return api.Shutdown200JSONResponse{
		Success: true,
		Message: &msg,
	}, nil
}

func (s *SimulatedController) GetAudioSinks(ctx context.Context, request api.GetAudioSinksRequestObject) (api.GetAudioSinksResponseObject, error) {
	return api.GetAudioSinks200JSONResponse{
		{Id: 55, Name: "alsa_output.pci-0000_01_00.1.hdmi-stereo", Description: "HDA NVidia Digital Stereo (HDMI)", Volume: 0.65, Muted: false, Default: true},
		{Id: 61, Name: "alsa_output.usb-Logitech_Z407.analog-stereo", Description: "Logi Z407 Analogue Stereo", Volume: 1.0, Muted: false, Default: false},
	}, nil
}

func (s *SimulatedController) SetAudioVolume(ctx context.Context, request api.SetAudioVolumeRequestObject) (api.SetAudioVolumeResponseObject, error) {
	if request.Body == nil || request.Body.Name == "" {
		return api.SetAudioVolume400JSONResponse{Code: 400, Error: "sink name is required"}, nil
	}
	log.Printf("[SIM] Audio set on sink %q", request.Body.Name)
	msg := "audio updated"
	return api.SetAudioVolume200JSONResponse{Success: true, Message: &msg}, nil
}

func (s *SimulatedController) GetLayouts(ctx context.Context, request api.GetLayoutsRequestObject) (api.GetLayoutsResponseObject, error) {
	s.mu.RLock()
	layouts := s.layouts.List()
	current := s.currentLayout
	s.mu.RUnlock()

	// Ensure we return an empty array instead of nil
	apiLayouts := layouts
	if apiLayouts == nil {
		apiLayouts = []api.Layout{}
	}

	return api.GetLayouts200JSONResponse{
		Layouts:       apiLayouts,
		CurrentLayout: current,
	}, nil
}

func (s *SimulatedController) SwitchLayout(ctx context.Context, request api.SwitchLayoutRequestObject) (api.SwitchLayoutResponseObject, error) {
	if request.Body == nil || request.Body.Layout == "" {
		return api.SwitchLayout400JSONResponse{Error: "layout name is required"}, nil
	}
	layoutName := request.Body.Layout

	s.mu.Lock()
	defer s.mu.Unlock()

	matches := s.layouts.FindByIDOrAlias(layoutName)
	if len(matches) == 0 {
		return api.SwitchLayout404JSONResponse{
			Code:  http.StatusNotFound,
			Error: fmt.Sprintf("layout %q not found", layoutName),
		}, nil
	}
	if len(matches) > 1 {
		return api.SwitchLayout400JSONResponse{
			Code:  http.StatusBadRequest,
			Error: fmt.Sprintf("ambiguous layout reference %q", layoutName),
		}, nil
	}

	s.currentLayout = matches[0].Id
	s.updateMonitorStates()

	log.Printf("[SIM] Switched to layout %q", s.currentLayout)

	msg := fmt.Sprintf("Switched to layout %q", matches[0].Name)
	return api.SwitchLayout200JSONResponse{
		Success:       true,
		CurrentLayout: s.currentLayout,
		Message:       &msg,
	}, nil
}

func (s *SimulatedController) GetCurrentLayout(ctx context.Context, request api.GetCurrentLayoutRequestObject) (api.GetCurrentLayoutResponseObject, error) {
	s.mu.RLock()
	current := s.currentLayout
	s.mu.RUnlock()

	return api.GetCurrentLayout200JSONResponse{
		Success:       true,
		CurrentLayout: current,
	}, nil
}

func (s *SimulatedController) SaveCurrentLayout(ctx context.Context, request api.SaveCurrentLayoutRequestObject) (api.SaveCurrentLayoutResponseObject, error) {
	if request.Body == nil || request.Body.Name == "" {
		return api.SaveCurrentLayout400JSONResponse{Error: "name is required"}, nil
	}
	name := request.Body.Name
	id := ""
	if request.Body.Id != nil {
		id = *request.Body.Id
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if id == "" {
		id = slugify(name)
	}

	// Build monitors from current active monitors
	var monitors []api.LayoutMonitor
	for _, m := range s.monitors {
		if m.Active != nil {
			monitors = append(monitors, api.LayoutMonitor{
				Edid:        m.Edid,
				Name:        m.Name,
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
		Monitors: monitors,
	}
	s.layouts.Set(layout)

	log.Printf("[SIM] Saved layout %q (%s)", layout.Name, layout.Id)

	msg := fmt.Sprintf("Saved layout %q", layout.Name)
	return api.SaveCurrentLayout200JSONResponse{
		Success: true,
		Layout:  layout,
		Message: &msg,
	}, nil
}

func (s *SimulatedController) RemoveLayout(ctx context.Context, request api.RemoveLayoutRequestObject) (api.RemoveLayoutResponseObject, error) {
	if request.Body == nil || request.Body.Layout == "" {
		return api.RemoveLayout400JSONResponse{Error: "layout name is required"}, nil
	}
	layoutName := request.Body.Layout

	s.mu.Lock()
	defer s.mu.Unlock()

	matches := s.layouts.FindByIDOrAlias(layoutName)
	if len(matches) == 0 {
		return api.RemoveLayout404JSONResponse{
			Code:  http.StatusNotFound,
			Error: fmt.Sprintf("layout %q not found", layoutName),
		}, nil
	}

	for _, m := range matches {
		s.layouts.Delete(m.Id)
		log.Printf("[SIM] Removed layout %q (%s)", m.Name, m.Id)
	}

	msg := fmt.Sprintf("Removed layout %q", layoutName)
	return api.RemoveLayout200JSONResponse{
		Success: true,
		Message: &msg,
	}, nil
}

func (s *SimulatedController) GetMonitors(ctx context.Context, request api.GetMonitorsRequestObject) (api.GetMonitorsResponseObject, error) {
	s.mu.RLock()
	// Return a copy
	monitors := make([]api.Monitor, len(s.monitors))
	copy(monitors, s.monitors)
	s.mu.RUnlock()

	// Ensure we return an empty array instead of nil
	apiMonitors := make([]api.Monitor, 0, len(monitors))
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

	return api.GetMonitors200JSONResponse(apiMonitors), nil
}

// cancelTrackpads closes all active trackpad WebSocket connections. Must be called with s.mu held.
func (s *SimulatedController) cancelTrackpads() {
	for _, cancel := range s.trackpadCancels {
		cancel()
	}
	s.trackpadCancels = nil
}

// --- Trackpad handler ---

// computeScreenBounds returns the bounding box of all active monitors.
func (s *SimulatedController) computeScreenBounds() (minX, minY, maxX, maxY int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	first := true
	for _, m := range s.monitors {
		if m.Active != nil {
			left := m.Active.PositionX
			top := m.Active.PositionY
			right := left + m.Active.Width
			bottom := top + m.Active.Height
			if first {
				minX, minY, maxX, maxY = left, top, right, bottom
				first = false
			} else {
				if left < minX {
					minX = left
				}
				if top < minY {
					minY = top
				}
				if right > maxX {
					maxX = right
				}
				if bottom > maxY {
					maxY = bottom
				}
			}
		}
	}
	if first {
		maxX = 1920
		maxY = 1080
	}
	return
}

// TopologyMouse wraps a MouseController to clamp movement to active monitors.
type TopologyMouse struct {
	input.MouseController
	s            *SimulatedController
	x, y         int
	fracX, fracY float64
}

func (m *TopologyMouse) GetPosition() (int, int, error) {
	return m.x, m.y, nil
}

func (m *TopologyMouse) MoveRelative(dx, dy float64) error {
	m.fracX += dx
	m.fracY += dy

	intX := int(m.fracX)
	intY := int(m.fracY)

	if intX == 0 && intY == 0 {
		return nil
	}

	m.fracX -= float64(intX)
	m.fracY -= float64(intY)

	return m.Move(intX, intY)
}

func (m *TopologyMouse) MoveTo(x, y int) error {
	m.s.mu.RLock()
	defer m.s.mu.RUnlock()
	if m.isValid(x, y) {
		m.x, m.y = x, y
		return m.MouseController.MoveTo(x, y)
	}
	return errors.New("invalid position")
}

func (m *TopologyMouse) Move(dx, dy int) error {
	m.s.mu.RLock()
	defer m.s.mu.RUnlock()

	// If current position became invalid (e.g. monitor removed), reset to nearest valid position
	if !m.isValid(m.x, m.y) {
		bestDist := int(^uint(0) >> 1)
		bestX, bestY := m.x, m.y
		found := false

		for _, mon := range m.s.monitors {
			if mon.Active == nil {
				continue
			}

			// Clamp m.x, m.y to this monitor's bounds
			minX, minY := mon.Active.PositionX, mon.Active.PositionY
			maxX, maxY := minX+mon.Active.Width-1, minY+mon.Active.Height-1

			cx := m.x
			if cx < minX {
				cx = minX
			} else if cx > maxX {
				cx = maxX
			}

			cy := m.y
			if cy < minY {
				cy = minY
			} else if cy > maxY {
				cy = maxY
			}

			// Distance squared
			dist := (m.x-cx)*(m.x-cx) + (m.y-cy)*(m.y-cy)

			if dist < bestDist {
				bestDist = dist
				bestX, bestY = cx, cy
				found = true
			}
		}

		if found {
			m.x, m.y = bestX, bestY
		}
	}

	// Try moving X and Y
	nextX, nextY := m.x+dx, m.y+dy
	if m.isValid(nextX, nextY) {
		m.x, m.y = nextX, nextY
		m.MouseController.MoveTo(m.x, m.y)
		return nil
	}

	// Try moving only X
	if m.isValid(nextX, m.y) {
		m.x = nextX
		m.MouseController.MoveTo(m.x, m.y)
		return nil
	}

	// Try moving only Y
	if m.isValid(m.x, nextY) {
		m.y = nextY
		m.MouseController.MoveTo(m.x, m.y)
		return nil
	}

	return errors.New("blocked")
}

func (m *TopologyMouse) isValid(x, y int) bool {
	for _, mon := range m.s.monitors {
		if mon.Active != nil {
			if x >= mon.Active.PositionX && x < mon.Active.PositionX+mon.Active.Width &&
				y >= mon.Active.PositionY && y < mon.Active.PositionY+mon.Active.Height {
				return true
			}
		}
	}
	return false
}

// ConnectTrackpad implements api.StrictServerInterface
// handleTrackpadWebSocket handles WebSocket connections for the simulated trackpad
func (s *SimulatedController) handleTrackpadWebSocket(w http.ResponseWriter, r *http.Request) {
	// Check if agent is online
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	if state != agentOnline {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":  http.StatusBadGateway,
			"error": "Bad Gateway (Client unreachable)",
		})
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Printf("[SIM] Trackpad WebSocket accept error: %v", err)
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	s.mu.Lock()
	s.trackpadCancels = append(s.trackpadCancels, cancel)
	s.mu.Unlock()

	minX, minY, maxX, maxY := s.computeScreenBounds()
	// Use large bounds for baseMouse so it doesn't interfere with TopologyMouse clamping
	baseMouse := input.NewSimulatedMouse((minX+maxX)/2, (minY+maxY)/2, -100000, -100000, 100000, 100000)
	keyboard := input.NewSimulatedKeyboard()

	mouse := &TopologyMouse{
		MouseController: baseMouse,
		s:               s,
		x:               (minX + maxX) / 2,
		y:               (minY + maxY) / 2,
	}

	var latestX, latestY atomic.Int32
	var posReady atomic.Bool

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
				keyboard.KeyUp(k, nil)
				delete(currentModifiers, k)
			}
		}
		for k := range target {
			if !currentModifiers[k] {
				keyboard.KeyDown(k, nil)
				currentModifiers[k] = true
			}
		}
	}

	log.Printf("[SIM] Trackpad connected")

	// Position update sender (60Hz), skip if position unchanged
	var lastSentX, lastSentY atomic.Int32
	lastSentX.Store(int32((minX+maxX)/2 + 1)) // ensure first send
	lastSentY.Store(int32((minY+maxY)/2 + 1))
	go func() {
		ticker := time.NewTicker(16 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var x, y int32
				if posReady.Swap(false) {
					x = latestX.Load()
					y = latestY.Load()
				} else {
					// Poll current position to catch external movement
					s.mu.RLock()
					mx, my, err := mouse.GetPosition()
					s.mu.RUnlock()
					if err != nil {
						continue
					}
					x, y = int32(mx), int32(my)
				}

				if x == lastSentX.Load() && y == lastSentY.Load() {
					continue
				}
				lastSentX.Store(x)
				lastSentY.Store(y)

				xInt, yInt := int(x), int(y)
				msg := api.TrackpadMessage{}
				msg.FromTrackpadMessageMousePositionUpdate(api.TrackpadMessageMousePositionUpdate{
					X: xInt,
					Y: yInt,
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
			mouse.MoveRelative(v.Dx, v.Dy)
		case api.TrackpadMessageMouseClick:
			updateModifiers(v.Modifiers)
			baseMouse.Click(input.ParseMouseButton(string(v.Btn)))
		case api.TrackpadMessageMouseDown:
			updateModifiers(v.Modifiers)
			baseMouse.ButtonDown(input.ParseMouseButton(string(v.Btn)))
		case api.TrackpadMessageMouseUp:
			updateModifiers(v.Modifiers)
			baseMouse.ButtonUp(input.ParseMouseButton(string(v.Btn)))
		case api.TrackpadMessageMouseScroll:
			precise := v.Precise != nil && *v.Precise
			baseMouse.Scroll(int(v.Dx), int(v.Dy), precise)
		case api.TrackpadMessageKeyDown:
			updateModifiers(v.Modifiers)
			keyboard.KeyDown(v.Key, nil)
		case api.TrackpadMessageKeyUp:
			updateModifiers(v.Modifiers)
			keyboard.KeyUp(v.Key, nil)
		case api.TrackpadMessageMouseMoveTo:
			mouse.MoveTo(v.X, v.Y)
		}
	}

	log.Printf("[SIM] Trackpad disconnected")
}

// ConnectTrackpad implements api.StrictServerInterface (stub, actual handler registered separately)
func (s *SimulatedController) ConnectTrackpad(ctx context.Context, request api.ConnectTrackpadRequestObject) (api.ConnectTrackpadResponseObject, error) {
	// This should never be called since we register the handler directly
	// But we need it to satisfy the StrictServerInterface
	return nil, fmt.Errorf("WebSocket handler should be called directly")
}

// --- Admin endpoints ---

func (s *SimulatedController) SimReset(ctx context.Context, request api.SimResetRequestObject) (api.SimResetResponseObject, error) {
	s.mu.Lock()
	if s.bootTimer != nil {
		s.bootTimer.Stop()
		s.bootTimer = nil
	}
	s.state = agentOffline
	s.cancelTrackpads()
	s.mu.Unlock()

	log.Printf("[SIM] Agent reset to OFFLINE")
	return api.SimReset200JSONResponse{State: "offline"}, nil
}

func (s *SimulatedController) SimState(ctx context.Context, request api.SimStateRequestObject) (api.SimStateResponseObject, error) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	return api.SimState200JSONResponse{State: state.String()}, nil
}

func (s *SimulatedController) SimSetState(ctx context.Context, request api.SimSetStateRequestObject) (api.SimSetStateResponseObject, error) {
	if request.Body == nil || request.Body.State == "" {
		return api.SimSetState404JSONResponse{Error: "state is required"}, nil
	}
	reqState := request.Body.State

	s.mu.Lock()
	// Cancel any pending boot timer
	if s.bootTimer != nil {
		s.bootTimer.Stop()
		s.bootTimer = nil
	}

	switch reqState {
	case "offline":
		s.state = agentOffline
		s.cancelTrackpads()
	case "booting":
		s.state = agentBooting
		s.cancelTrackpads()
	case "online":
		s.state = agentOnline
	default:
		s.mu.Unlock()
		return api.SimSetState404JSONResponse{
			Code:  http.StatusNotFound,
			Error: fmt.Sprintf("invalid state %q", reqState),
		}, nil
	}
	state := s.state
	s.mu.Unlock()

	log.Printf("[SIM] Agent state set to %s", state)
	return api.SimSetState200JSONResponse{State: state.String()}, nil
}

// Start starts the simulated HTTP server.
func (s *SimulatedController) Start() error {
	s.server = &http.Server{
		Addr:         s.controllerCfg.ListenAddress,
		Handler:      common.LoggingMiddleware(s.router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Print startup banner
	var edids []string
	for _, m := range s.monitors {
		edids = append(edids, m.Edid)
	}
	log.Println()
	log.Println("=== SIMULATED CONTROLLER ===")
	log.Printf("Listen:   %s\n", s.controllerCfg.ListenAddress)
	log.Printf("State:    %s\n", s.state)
	log.Printf("Boot delay: %s\n", s.bootDelay)
	log.Printf("Layouts:  %d\n", len(s.layouts.List()))
	log.Printf("Monitors: %s\n", strings.Join(edids, ", "))
	log.Println()
	log.Println("Admin endpoints:")
	log.Println("  POST /api/sim/reset     - Reset agent to offline")
	log.Println("  GET  /api/sim/state     - Get current state")
	log.Println("  POST /api/sim/set-state - Set state (offline/booting/online)")
	log.Println("========================")
	log.Println()

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Simulated controller starting on http://%s", s.controllerCfg.ListenAddress)
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down simulated controller...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// slugify converts a string into a URL-friendly slug.
func slugify(input string) string {
	slug := regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(strings.ToLower(input), "-")
	return strings.Trim(slug, "-")
}
