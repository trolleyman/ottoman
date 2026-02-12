package client

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
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
	"github.com/trolleyman/ottoman/internal/common"
	"github.com/trolleyman/ottoman/internal/config"
	"github.com/trolleyman/ottoman/internal/display"
	"github.com/trolleyman/ottoman/internal/input"
)

// Client is the display control agent running on the desktop
type Client struct {
	config        *Config
	configPath    string
	router        *http.ServeMux
	server        *http.Server
	layouts       *display.Layouts
	displayMgr    display.Manager
	mouse         input.MouseController
	startTime     time.Time
	currentLayout string
}

// New creates a new client instance
func New(cfg *Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	// Load layouts from config
	store := display.NewLayoutsFromSlice(cfg.Layouts)

	// Create display manager
	mgr, err := display.NewManager(store)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create display manager")
	}

	// Create mouse controller
	mouse, err := input.NewMouseController()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create mouse controller")
	}

	c := &Client{
		config:     cfg,
		configPath: config.ConfigPath(),
		layouts:    store,
		displayMgr: mgr,
		mouse:      mouse,
		startTime:  time.Now(),
	}

	if err := c.setupRoutes(); err != nil {
		return nil, err
	}

	return c, nil
}

// setupRoutes configures HTTP routes
func (c *Client) setupRoutes() error {
	c.router = http.NewServeMux()

	// Health check (no auth)
	c.router.HandleFunc("GET /health", c.handleHealth)
	c.router.HandleFunc("GET /api/status", c.handleStatus)

	// Auth endpoints (no auth required for login/logout)
	c.router.HandleFunc("POST /api/auth", c.handleAuth)
	c.router.HandleFunc("POST /api/auth/logout", c.handleLogout)
	c.router.HandleFunc("GET /api/auth/check", c.requireAuth(c.handleAuthCheck))

	// Display control
	c.router.HandleFunc("GET /api/layouts", c.requireAuth(c.handleListLayouts))
	c.router.HandleFunc("POST /api/layouts/switch", c.requireAuth(c.handleSwitchLayout))
	c.router.HandleFunc("GET /api/layouts/current", c.requireAuth(c.handleCurrentLayout))
	c.router.HandleFunc("POST /api/layouts/save-current", c.requireAuth(c.handleSaveCurrentLayout))
	c.router.HandleFunc("POST /api/layouts/remove", c.requireAuth(c.handleRemoveLayout))

	// Monitor info
	c.router.HandleFunc("GET /api/monitors", c.requireAuth(c.handleListMonitors))

	// Shutdown
	c.router.HandleFunc("POST /api/shutdown", c.requireAuth(c.handleShutdown))

	// Trackpad (WebSocket)
	c.router.HandleFunc("GET /api/trackpad", c.requireAuth(c.handleTrackpad))

	if err := common.SetupSPAHandler(c.router); err != nil {
		return errors.Wrap(err, "failed to create SPA handler")
	}

	return nil
}

// requireAuth wraps a handler with authentication
func (c *Client) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c.config.AuthToken == "" {
			next(w, r)
			return
		}

		// Check Authorization: Bearer header
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if subtle.ConstantTimeCompare([]byte(token), []byte(c.config.AuthToken)) == 1 {
				next(w, r)
				return
			}
		}

		// Check ottoman_token cookie
		if cookie, err := r.Cookie("ottoman_token"); err == nil {
			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(c.config.AuthToken)) == 1 {
				next(w, r)
				return
			}
		}

		w.Header().Set("WWW-Authenticate", `Bearer realm="ottoman"`)
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
	}
}

// handleHealth returns a simple health check
func (c *Client) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleAuth validates a token and sets an auth cookie
func (c *Client) handleAuth(w http.ResponseWriter, r *http.Request) {
	var req common.AuthRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if subtle.ConstantTimeCompare([]byte(req.Token), []byte(c.config.AuthToken)) != 1 {
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

// handleLogout clears the auth cookie
func (c *Client) handleLogout(w http.ResponseWriter, r *http.Request) {
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

// handleAuthCheck returns whether the request is authenticated
func (c *Client) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	common.WriteJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

// handleStatus returns detailed status
func (c *Client) handleStatus(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(c.startTime).Round(time.Second).String()
	common.WriteJSON(w, http.StatusOK, common.StatusResponse{
		Status:  "ok",
		Version: "dev",
		Uptime:  uptime,
	})
}

// handleListLayouts returns available display layouts
func (c *Client) handleListLayouts(w http.ResponseWriter, r *http.Request) {
	allLayouts := c.layouts.List()

	// Update current layout from display manager to ensure it's fresh
	if monitors, err := c.displayMgr.ListMonitors(); err == nil {
		if current, ok := c.layouts.GetClosest(monitors); ok {
			c.currentLayout = current
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
		return allLayouts[i].ID < allLayouts[j].ID
	})

	common.WriteJSON(w, http.StatusOK, common.ListLayoutsResponse{
		Layouts:       allLayouts,
		CurrentLayout: c.currentLayout,
	})
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

// handleSwitchLayout switches to a named layout
func (c *Client) handleSwitchLayout(w http.ResponseWriter, r *http.Request) {
	var req common.SwitchLayoutRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Layout == "" {
		common.WriteError(w, http.StatusBadRequest, "layout name is required")
		return
	}

	log.Printf("Switching to layout: %s", req.Layout)

	layouts := c.layouts.FindByIDOrAlias(req.Layout)
	if len(layouts) == 0 {
		common.WriteError(w, http.StatusNotFound, fmt.Sprintf("layout %q not found", req.Layout))
		return
	}
	if len(layouts) > 1 {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("layout %q is ambiguous", req.Layout))
		return
	}

	layout := layouts[0]

	if err := c.displayMgr.ApplyLayoutConfig(layout); err != nil {
		log.Printf("Failed to apply layout: %v", err)
		common.WriteJSON(w, http.StatusInternalServerError, common.SwitchLayoutResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.currentLayout = req.Layout

	common.WriteJSON(w, http.StatusOK, common.SwitchLayoutResponse{
		Success:       true,
		CurrentLayout: req.Layout,
		Message:       fmt.Sprintf("Switched to layout: %s", req.Layout),
	})
}

// handleCurrentLayout returns the current layout
func (c *Client) handleCurrentLayout(w http.ResponseWriter, r *http.Request) {
	var currentLayout string
	monitors, err := c.displayMgr.ListMonitors()
	if err != nil {
		log.Printf("Failed to list monitors: %v", err)
		currentLayout = c.currentLayout
	} else {
		if layout, ok := c.layouts.GetClosest(monitors); ok {
			currentLayout = layout
		}
	}

	if currentLayout == "" {
		currentLayout = c.currentLayout
	}

	common.WriteJSON(w, http.StatusOK, common.SwitchLayoutResponse{
		Success:       true,
		CurrentLayout: currentLayout,
	})
}

// handleSaveCurrentLayout saves the current layout
func (c *Client) handleSaveCurrentLayout(w http.ResponseWriter, r *http.Request) {
	var req common.SaveLayoutRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Generate ID if not provided
	if req.ID == "" {
		req.ID = slugify(req.Name)
	}

	// Get current monitor state to save
	monitors, err := c.displayMgr.ListMonitors()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to get current monitors")
		return
	}

	// Convert MonitorInfo to Monitor config
	var monitorConfigs []common.Monitor
	for _, m := range monitors {
		if m.Active != nil {
			monitorConfigs = append(monitorConfigs, common.Monitor{
				Name:        m.Name,
				EDID:        m.EDID,
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
		ID:       req.ID,
		Name:     req.Name,
		Emoji:    req.Emoji,
		Monitors: monitorConfigs,
	}

	c.layouts.Set(layout)

	// Save to config file
	if err := c.saveLayouts(); err != nil {
		log.Printf("Failed to save layouts: %v", err)
		common.WriteError(w, http.StatusInternalServerError, "failed to save layout")
		return
	}

	common.WriteJSON(w, http.StatusOK, common.SaveLayoutResponse{
		Success: true,
		Layout:  &layout,
	})
}

// handleRemoveLayout removes a layout by name/ID
func (c *Client) handleRemoveLayout(w http.ResponseWriter, r *http.Request) {
	var req common.RemoveLayoutRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Layout == "" {
		common.WriteError(w, http.StatusBadRequest, "layout name is required")
		return
	}

	if _, ok := c.layouts.Get(req.Layout); !ok {
		common.WriteError(w, http.StatusNotFound, fmt.Sprintf("layout %q not found", req.Layout))
		return
	}

	log.Printf("Removing layout: %s", req.Layout)
	c.layouts.Delete(req.Layout)

	if c.currentLayout == req.Layout {
		c.currentLayout = ""
	}

	if err := c.saveLayouts(); err != nil {
		log.Printf("Failed to save layouts: %v", err)
		common.WriteError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	common.WriteJSON(w, http.StatusOK, common.RemoveLayoutResponse{
		Success: true,
		Message: fmt.Sprintf("Removed layout: %s", req.Layout),
	})
}

// handleListMonitors returns connected monitor information
func (c *Client) handleListMonitors(w http.ResponseWriter, r *http.Request) {
	monitors, err := c.displayMgr.ListMonitors()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, monitors)
}

// handleShutdown initiates an OS shutdown
func (c *Client) handleShutdown(w http.ResponseWriter, r *http.Request) {
	log.Println("Shutdown requested via API")

	// Respond before shutting down
	common.WriteJSON(w, http.StatusOK, common.ShutdownResponse{
		Success: true,
		Message: "Shutdown initiated",
	})

	// Flush response, then shut down after a brief delay
	go func() {
		time.Sleep(1 * time.Second)

		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("shutdown", "/s", "/t", "0")
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
}

// handleTrackpad handles WebSocket connections for trackpad input.
func (c *Client) handleTrackpad(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Printf("Trackpad WebSocket accept error: %v", err)
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Throttled position sender: latest position stored atomically, sent at 10Hz
	var latestX, latestY atomic.Int32
	var posReady atomic.Bool

	sensitivity := c.config.TrackpadSensitivity
	if sensitivity <= 0 {
		sensitivity = 1.5
	}
	friction := c.config.TrackpadFriction
	if friction <= 0 {
		friction = 0.92
	}

	engine := input.NewInertiaEngine(c.mouse, sensitivity, friction, func(x, y int) {
		latestX.Store(int32(x))
		latestY.Store(int32(y))
		posReady.Store(true)
	})

	// Position update sender goroutine (60Hz), skip if position unchanged
	var lastSentX, lastSentY atomic.Int32
	lastSentX.Store(latestX.Load() + 1) // ensure first send
	lastSentY.Store(latestY.Load() + 1)
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
}

// Run starts the client
func Run(config *Config) error {
	client, err := New(config)
	if err != nil {
		return err
	}

	return client.Start()
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
func (c *Client) Start() error {
	c.server = &http.Server{
		Addr:         c.config.ListenAddr,
		Handler:      common.LoggingMiddleware(c.router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Client starting at http://%s", c.config.ListenAddr)
		if err := c.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down client...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return c.server.Shutdown(ctx)
}

// CheckStatus checks if a client is reachable
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

// saveLayouts saves the current layouts to the config file
func (c *Client) saveLayouts() error {
	// Update config with current layouts
	c.config.Layouts = c.layouts.ToSlice()

	// Save client config only
	return config.SaveClient(c.config, c.configPath)
}
