package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
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
	"github.com/trolleyman/ottoman/internal/common"
	"github.com/trolleyman/ottoman/internal/config"
	"github.com/trolleyman/ottoman/internal/display"
	"github.com/trolleyman/ottoman/internal/input"
)

type clientState int

const (
	clientOffline clientState = iota
	clientBooting
	clientOnline
)

func (s clientState) String() string {
	switch s {
	case clientOffline:
		return "offline"
	case clientBooting:
		return "booting"
	case clientOnline:
		return "online"
	default:
		return "unknown"
	}
}

// SimulatedServer serves the real frontend with mocked API endpoints for testing WoL flows.
type SimulatedServer struct {
	serverCfg *config.ServerConfig
	router    *http.ServeMux
	server    *http.Server
	startTime time.Time

	mu        sync.RWMutex
	state     clientState
	bootTimer *time.Timer
	bootDelay time.Duration

	wakeTargets     map[string]config.WakeTarget
	layouts         *display.Layouts
	currentLayout   string
	monitors        []display.MonitorInfo
	trackpadCancels []context.CancelFunc
}

// RunSimulated creates and starts a simulated server.
func RunSimulated(serverCfg *config.ServerConfig, clientCfg *config.ClientConfig, bootDelay time.Duration, startOnline bool) error {
	sim, err := NewSimulated(serverCfg, clientCfg, bootDelay, startOnline)
	if err != nil {
		return err
	}
	return sim.Start()
}

// NewSimulated creates a new simulated server instance.
func NewSimulated(serverCfg *config.ServerConfig, clientCfg *config.ClientConfig, bootDelay time.Duration, startOnline bool) (*SimulatedServer, error) {
	if serverCfg.AuthToken == "" && serverCfg.Username == "" {
		return nil, errors.New("server config requires auth_token or username")
	}

	s := &SimulatedServer{
		serverCfg:   serverCfg,
		bootDelay:   bootDelay,
		startTime:   time.Now(),
		wakeTargets: make(map[string]config.WakeTarget),
	}

	if startOnline {
		s.state = clientOnline
	}

	// Index wake targets by name
	for _, target := range serverCfg.WakeTargets {
		s.wakeTargets[strings.ToLower(target.Name)] = target
	}

	// Load layouts from client config
	s.layouts = display.NewLayoutsFromSlice(clientCfg.Layouts)

	// Set initial current layout to first layout (sorted by alias)
	if len(clientCfg.Layouts) > 0 {
		s.currentLayout = clientCfg.Layouts[0].ID
	}

	// Derive monitors from layouts
	s.monitors = deriveMonitors(clientCfg.Layouts)

	// Update monitor active states to match current layout
	s.updateMonitorStates()

	if err := s.setupRoutes(); err != nil {
		return nil, err
	}

	return s, nil
}

// deriveMonitors collects unique monitors by EDID across all layouts.
func deriveMonitors(layouts []common.Layout) []display.MonitorInfo {
	seen := make(map[string]bool)
	var monitors []display.MonitorInfo

	for _, layout := range layouts {
		for _, m := range layout.Monitors {
			if m.EDID == "" || seen[m.EDID] {
				continue
			}
			seen[m.EDID] = true

			// Extract manufacturer from EDID (format "MFR:PRODUCT")
			manufacturer := m.EDID
			if idx := strings.Index(m.EDID, ":"); idx >= 0 {
				manufacturer = m.EDID[:idx]
			}

			monitors = append(monitors, display.MonitorInfo{
				EDID:         m.EDID,
				Port:         m.Port,
				Name:         m.Name,
				Manufacturer: manufacturer,
			})
		}
	}
	return monitors
}

// updateMonitorStates sets Active on each monitor based on the current layout.
func (s *SimulatedServer) updateMonitorStates() {
	layout, ok := s.layouts.Get(s.currentLayout)
	if !ok {
		// No current layout — all monitors inactive
		for i := range s.monitors {
			s.monitors[i].Active = nil
		}
		return
	}

	// Build lookup by EDID from the current layout
	layoutMonitors := make(map[string]common.Monitor)
	for _, m := range layout.Monitors {
		layoutMonitors[m.EDID] = m
	}

	for i := range s.monitors {
		if lm, ok := layoutMonitors[s.monitors[i].EDID]; ok {
			s.monitors[i].Active = &display.ConnectedInfo{
				Width:       lm.Width,
				Height:      lm.Height,
				RefreshRate: lm.RefreshRate,
				PositionX:   lm.PositionX,
				PositionY:   lm.PositionY,
				Primary:     lm.Primary,
				Model:       s.monitors[i].Name,
			}
		} else {
			s.monitors[i].Active = nil
		}
	}
}

func (s *SimulatedServer) setupRoutes() error {
	s.router = http.NewServeMux()

	// Health check (no auth required)
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("GET /api/status", s.handleStatus)

	// Auth endpoints
	s.router.HandleFunc("POST /api/auth", s.handleAuth)
	s.router.HandleFunc("POST /api/auth/logout", s.handleLogout)
	s.router.HandleFunc("GET /api/auth/check", s.requireAuth(s.handleAuthCheck))

	// Wake-on-LAN (simulated)
	s.router.HandleFunc("POST /api/wake", s.requireAuth(s.handleWake))
	s.router.HandleFunc("GET /api/wake/targets", s.requireAuth(s.handleListWakeTargets))

	// Display control (simulated)
	s.router.HandleFunc("GET /api/layouts", s.requireAuth(s.handleListLayouts))
	s.router.HandleFunc("POST /api/layouts/switch", s.requireAuth(s.handleSwitchLayout))
	s.router.HandleFunc("GET /api/layouts/current", s.requireAuth(s.handleCurrentLayout))
	s.router.HandleFunc("POST /api/layouts/save-current", s.requireAuth(s.handleSaveCurrentLayout))
	s.router.HandleFunc("POST /api/layouts/remove", s.requireAuth(s.handleRemoveLayout))
	s.router.HandleFunc("GET /api/monitors", s.requireAuth(s.handleListMonitors))

	// Shutdown (simulated)
	s.router.HandleFunc("POST /api/shutdown", s.requireAuth(s.handleShutdown))

	// Client status (simulated)
	s.router.HandleFunc("GET /api/client/status", s.requireAuth(s.handleClientStatus))

	// Trackpad (WebSocket, simulated)
	s.router.HandleFunc("GET /api/trackpad", s.requireAuth(s.handleTrackpad))

	// Admin endpoints (no auth, for dev convenience)
	s.router.HandleFunc("POST /api/sim/reset", s.handleSimReset)
	s.router.HandleFunc("GET /api/sim/state", s.handleSimState)
	s.router.HandleFunc("POST /api/sim/set-state", s.handleSimSetState)

	if err := common.SetupSPAHandler(s.router); err != nil {
		return errors.Wrap(err, "failed to setup SPA handler")
	}
	return nil
}

// requireAuth wraps a handler with authentication (same logic as real server).
func (s *SimulatedServer) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")

		// Token-based auth
		if s.serverCfg.AuthToken != "" {
			if token, ok := strings.CutPrefix(auth, "Bearer "); ok {
				if subtle.ConstantTimeCompare([]byte(token), []byte(s.serverCfg.AuthToken)) == 1 {
					next(w, r)
					return
				}
			}
		}

		// Check ottoman_token cookie
		if cookie, err := r.Cookie("ottoman_token"); err == nil {
			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.serverCfg.AuthToken)) == 1 {
				next(w, r)
				return
			}
		}

		// Basic auth
		if s.serverCfg.Username != "" {
			username, password, ok := r.BasicAuth()
			if ok && username == s.serverCfg.Username {
				if subtle.ConstantTimeCompare([]byte(password), []byte(s.serverCfg.PasswordHash)) == 1 {
					next(w, r)
					return
				}
			}
		}

		w.Header().Set("WWW-Authenticate", `Bearer realm="ottoman"`)
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
	}
}

// clientOnlineOrError checks if the simulated client is online. If not, writes a 502 error and returns false.
func (s *SimulatedServer) clientOnlineOrError(w http.ResponseWriter, path string) bool {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	if state == clientOnline {
		return true
	}

	errMsg := fmt.Sprintf(
		`Get "http://%s%s": dial tcp %s: connectex: No connection could be made because the target machine actively refused it.`,
		s.serverCfg.ClientAddr, path, s.serverCfg.ClientAddr,
	)
	common.WriteError(w, http.StatusBadGateway, errMsg)
	return false
}

// --- Standard handlers (identical to real server) ---

func (s *SimulatedServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *SimulatedServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startTime).Round(time.Second).String()
	common.WriteJSON(w, http.StatusOK, common.StatusResponse{
		Status:  "ok",
		Version: "simulated",
		Uptime:  uptime,
	})
}

func (s *SimulatedServer) handleAuth(w http.ResponseWriter, r *http.Request) {
	var req common.AuthRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if subtle.ConstantTimeCompare([]byte(req.Token), []byte(s.serverCfg.AuthToken)) != 1 {
		common.WriteJSON(w, http.StatusUnauthorized, common.AuthResponse{
			Success: false,
			Message: "invalid token",
		})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "ottoman_token",
		Value:    req.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	common.WriteJSON(w, http.StatusOK, common.AuthResponse{
		Success: true,
	})
}

func (s *SimulatedServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "ottoman_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	common.WriteJSON(w, http.StatusOK, common.AuthResponse{
		Success: true,
	})
}

func (s *SimulatedServer) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	common.WriteJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

// --- Simulated WoL handlers ---

func (s *SimulatedServer) handleWake(w http.ResponseWriter, r *http.Request) {
	var req common.WakeRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Find target
	s.mu.Lock()
	target, ok := s.wakeTargets[strings.ToLower(req.Target)]
	state := s.state

	var macAddr string
	if ok {
		macAddr = target.MACAddress
	} else {
		macAddr = req.Target
	}

	switch state {
	case clientOffline:
		s.state = clientBooting
		s.bootTimer = time.AfterFunc(s.bootDelay, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.state == clientBooting {
				s.state = clientOnline
				log.Printf("[SIM] Client is now ONLINE (boot complete after %s)", s.bootDelay)
			}
		})
		s.mu.Unlock()
		log.Printf("[SIM] WoL sent to %s — client will be online in %s", macAddr, s.bootDelay)
		common.WriteJSON(w, http.StatusOK, common.WakeResponse{
			Success: true,
			Message: fmt.Sprintf("Wake-on-LAN packet sent to %s", macAddr),
		})

	case clientBooting:
		s.mu.Unlock()
		log.Printf("[SIM] WoL sent to %s — already booting", macAddr)
		common.WriteJSON(w, http.StatusOK, common.WakeResponse{
			Success: true,
			Message: fmt.Sprintf("Wake-on-LAN packet sent to %s (already booting)", macAddr),
		})

	case clientOnline:
		s.mu.Unlock()
		log.Printf("[SIM] WoL sent to %s — already online", macAddr)
		common.WriteJSON(w, http.StatusOK, common.WakeResponse{
			Success: true,
			Message: fmt.Sprintf("Wake-on-LAN packet sent to %s (already online)", macAddr),
		})
	}
}

func (s *SimulatedServer) handleListWakeTargets(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	state := s.state
	targets := make([]config.WakeTarget, 0, len(s.wakeTargets))
	for _, target := range s.wakeTargets {
		targets = append(targets, target)
	}
	s.mu.RUnlock()

	type WakeTargetStatus struct {
		config.WakeTarget
		Status string `json:"status"`
	}

	status := "offline"
	if state == clientOnline {
		status = "online"
	}

	results := make([]WakeTargetStatus, len(targets))
	for i, t := range targets {
		results[i] = WakeTargetStatus{
			WakeTarget: t,
			Status:     status,
		}
	}

	common.WriteJSON(w, http.StatusOK, results)
}

// --- Simulated display handlers ---

func (s *SimulatedServer) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if !s.clientOnlineOrError(w, "/api/shutdown") {
		return
	}

	s.mu.Lock()
	if s.bootTimer != nil {
		s.bootTimer.Stop()
		s.bootTimer = nil
	}
	s.state = clientOffline
	s.mu.Unlock()

	log.Printf("[SIM] Client shut down via API — now OFFLINE")
	common.WriteJSON(w, http.StatusOK, common.ShutdownResponse{
		Success: true,
		Message: "Shutdown initiated",
	})
}

func (s *SimulatedServer) handleClientStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	if state == clientOnline {
		common.WriteJSON(w, http.StatusOK, common.StatusResponse{Status: "ok"})
	} else {
		common.WriteJSON(w, http.StatusOK, common.StatusResponse{Status: "unreachable"})
	}
}

func (s *SimulatedServer) handleListLayouts(w http.ResponseWriter, r *http.Request) {
	if !s.clientOnlineOrError(w, "/api/layouts") {
		return
	}

	s.mu.RLock()
	layouts := s.layouts.List()
	current := s.currentLayout
	s.mu.RUnlock()

	common.WriteJSON(w, http.StatusOK, common.ListLayoutsResponse{
		Layouts:       layouts,
		CurrentLayout: current,
	})
}

func (s *SimulatedServer) handleSwitchLayout(w http.ResponseWriter, r *http.Request) {
	if !s.clientOnlineOrError(w, "/api/layouts/switch") {
		return
	}

	var req common.SwitchLayoutRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	matches := s.layouts.FindByIDOrAlias(req.Layout)
	if len(matches) == 0 {
		common.WriteError(w, http.StatusNotFound, fmt.Sprintf("layout %q not found", req.Layout))
		return
	}
	if len(matches) > 1 {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("ambiguous layout reference %q", req.Layout))
		return
	}

	s.currentLayout = matches[0].ID
	s.updateMonitorStates()

	log.Printf("[SIM] Switched to layout %q", s.currentLayout)

	common.WriteJSON(w, http.StatusOK, common.SwitchLayoutResponse{
		Success:       true,
		CurrentLayout: s.currentLayout,
		Message:       fmt.Sprintf("Switched to layout %q", matches[0].Name),
	})
}

func (s *SimulatedServer) handleCurrentLayout(w http.ResponseWriter, r *http.Request) {
	if !s.clientOnlineOrError(w, "/api/layouts/current") {
		return
	}

	s.mu.RLock()
	current := s.currentLayout
	s.mu.RUnlock()

	common.WriteJSON(w, http.StatusOK, map[string]string{"current_layout": current})
}

func (s *SimulatedServer) handleSaveCurrentLayout(w http.ResponseWriter, r *http.Request) {
	if !s.clientOnlineOrError(w, "/api/layouts/save-current") {
		return
	}

	var req common.SaveLayoutRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := req.ID
	if id == "" {
		id = slugify(req.Name)
	}

	// Build monitors from current active monitors
	var monitors []common.Monitor
	for _, m := range s.monitors {
		if m.Active != nil {
			monitors = append(monitors, common.Monitor{
				EDID:        m.EDID,
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

	layout := common.Layout{
		ID:       id,
		Name:     req.Name,
		Emoji:    req.Emoji,
		Monitors: monitors,
	}
	s.layouts.Set(layout)

	log.Printf("[SIM] Saved layout %q (%s)", layout.Name, layout.ID)

	common.WriteJSON(w, http.StatusOK, common.SaveLayoutResponse{
		Success: true,
		Layout:  &layout,
		Message: fmt.Sprintf("Saved layout %q", layout.Name),
	})
}

func (s *SimulatedServer) handleRemoveLayout(w http.ResponseWriter, r *http.Request) {
	if !s.clientOnlineOrError(w, "/api/layouts/remove") {
		return
	}

	var req common.RemoveLayoutRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	matches := s.layouts.FindByIDOrAlias(req.Layout)
	if len(matches) == 0 {
		common.WriteError(w, http.StatusNotFound, fmt.Sprintf("layout %q not found", req.Layout))
		return
	}

	for _, m := range matches {
		s.layouts.Delete(m.ID)
		log.Printf("[SIM] Removed layout %q (%s)", m.Name, m.ID)
	}

	common.WriteJSON(w, http.StatusOK, common.RemoveLayoutResponse{
		Success: true,
		Message: fmt.Sprintf("Removed layout %q", req.Layout),
	})
}

func (s *SimulatedServer) handleListMonitors(w http.ResponseWriter, r *http.Request) {
	if !s.clientOnlineOrError(w, "/api/monitors") {
		return
	}

	s.mu.RLock()
	// Return a copy
	monitors := make([]display.MonitorInfo, len(s.monitors))
	copy(monitors, s.monitors)
	s.mu.RUnlock()

	common.WriteJSON(w, http.StatusOK, monitors)
}

// --- Trackpad handler ---

// computeScreenBounds returns the bounding box of all active monitors.
func (s *SimulatedServer) computeScreenBounds() (minX, minY, maxX, maxY int) {
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

func (s *SimulatedServer) handleTrackpad(w http.ResponseWriter, r *http.Request) {
	if !s.clientOnlineOrError(w, "/api/trackpad") {
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
	mouse := input.NewSimulatedMouse((minX+maxX)/2, (minY+maxY)/2, minX, minY, maxX-1, maxY-1)

	var latestX, latestY atomic.Int32
	var posReady atomic.Bool

	engine := input.NewInertiaEngine(mouse, 1.5, 0.92, func(x, y int) {
		latestX.Store(int32(x))
		latestY.Store(int32(y))
		posReady.Store(true)
		log.Printf("[SIM] Pointer: (%d, %d)", x, y)
	})

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
				if !posReady.Swap(false) {
					continue
				}
				x := latestX.Load()
				y := latestY.Load()
				if x == lastSentX.Load() && y == lastSentY.Load() {
					continue
				}
				lastSentX.Store(x)
				lastSentY.Store(y)
				msg := common.TrackpadMessage{
					Type: "p",
					X:    int(x),
					Y:    int(y),
				}
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

		var msg common.TrackpadMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "s":
			touch := msg.Touch != nil && *msg.Touch
			engine.Start(touch)
		case "m":
			engine.Move(msg.DX, msg.DY)
		case "e":
			engine.End()
		}
	}

	log.Printf("[SIM] Trackpad disconnected")
}

// --- Admin endpoints ---

func (s *SimulatedServer) handleSimReset(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.bootTimer != nil {
		s.bootTimer.Stop()
		s.bootTimer = nil
	}
	s.state = clientOffline
	s.mu.Unlock()

	log.Printf("[SIM] Client reset to OFFLINE")
	common.WriteJSON(w, http.StatusOK, map[string]string{"state": "offline"})
}

func (s *SimulatedServer) handleSimState(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	common.WriteJSON(w, http.StatusOK, map[string]string{"state": state.String()})
}

func (s *SimulatedServer) handleSimSetState(w http.ResponseWriter, r *http.Request) {
	var req struct {
		State string `json:"state"`
	}
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.mu.Lock()
	// Cancel any pending boot timer
	if s.bootTimer != nil {
		s.bootTimer.Stop()
		s.bootTimer = nil
	}

	switch req.State {
	case "offline":
		s.state = clientOffline
	case "booting":
		s.state = clientBooting
	case "online":
		s.state = clientOnline
	default:
		s.mu.Unlock()
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid state %q (must be offline, booting, or online)", req.State))
		return
	}
	state := s.state
	s.mu.Unlock()

	log.Printf("[SIM] Client state set to %s", state)
	common.WriteJSON(w, http.StatusOK, map[string]string{"state": state.String()})
}

// Start starts the simulated HTTP server.
func (s *SimulatedServer) Start() error {
	s.server = &http.Server{
		Addr:         s.serverCfg.ListenAddr,
		Handler:      common.LoggingMiddleware(s.router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Print startup banner
	var edids []string
	for _, m := range s.monitors {
		edids = append(edids, m.EDID)
	}
	fmt.Println()
	fmt.Println("=== SIMULATED SERVER ===")
	fmt.Printf("Listen:   %s\n", s.serverCfg.ListenAddr)
	fmt.Printf("State:    %s\n", s.state)
	fmt.Printf("Boot delay: %s\n", s.bootDelay)
	fmt.Printf("Layouts:  %d\n", len(s.layouts.List()))
	fmt.Printf("Monitors: %s\n", strings.Join(edids, ", "))
	fmt.Println()
	fmt.Println("Admin endpoints:")
	fmt.Println("  POST /api/sim/reset     - Reset client to offline")
	fmt.Println("  GET  /api/sim/state     - Get current state")
	fmt.Println("  POST /api/sim/set-state - Set state (offline/booting/online)")
	fmt.Println("========================")
	fmt.Println()

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Simulated server starting on http://%s", s.serverCfg.ListenAddr)
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down simulated server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// slugify converts a string into a URL-friendly slug.
func slugify(input string) string {
	slug := regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(strings.ToLower(input), "-")
	return strings.Trim(slug, "-")
}
