package client

import (
	"context"
	"crypto/subtle"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/common"
	"github.com/trolleyman/ottoman/internal/config"
	"github.com/trolleyman/ottoman/internal/display"
)

// Client is the display control agent running on the desktop
type Client struct {
	config        *Config
	configPath    string
	router        *http.ServeMux
	server        *http.Server
	layouts       *display.Layouts
	displayMgr    display.Manager
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

	c := &Client{
		config:     cfg,
		configPath: config.ConfigPath(),
		layouts:    store,
		displayMgr: mgr,
		startTime:  time.Now(),
	}

	c.setupRoutes()

	return c, nil
}

// handleWebIndex serves the embedded HTML file
func (c *Client) handleWebIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Layouts</title>
	<script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="bg-gray-100 text-gray-800">
	<div class="container mx-auto p-4">
		<h1 class="text-2xl font-bold mb-4">Available Layouts</h1>
		<ul id="layout-list" class="list-disc pl-5">
			{{range .Layouts}}<li>{{.}}</li>{{else}}<li>No layouts</li>{{end}}
		</ul>
	</div>
</body>
</html>`

	// Retrieve layouts to render in the template
	allLayouts := c.layouts.List()
	var layouts []string
	for _, l := range allLayouts {
		layouts = append(layouts, l.Name)
	}

	t, err := template.New("index").Parse(tmpl)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to parse template")
		return
	}

	data := struct {
		Layouts []string
	}{Layouts: layouts}

	if err := t.Execute(w, data); err != nil {
		log.Printf("Failed to execute template: %v", err)
		// If headers already written, return; otherwise send error
		return
	}
}

// setupRoutes configures HTTP routes
func (c *Client) setupRoutes() {
	c.router = http.NewServeMux()

	// Web index
	c.router.HandleFunc("/", c.handleWebIndex)

	// Health check (no auth)
	c.router.HandleFunc("GET /health", c.handleHealth)
	c.router.HandleFunc("GET /api/status", c.handleStatus)

	// Display control
	c.router.HandleFunc("GET /api/layouts", c.requireAuth(c.handleListLayouts))
	c.router.HandleFunc("POST /api/layouts/switch", c.requireAuth(c.handleSwitchLayout))
	c.router.HandleFunc("GET /api/layouts/current", c.requireAuth(c.handleCurrentLayout))
	c.router.HandleFunc("POST /api/layouts/save-current", c.requireAuth(c.handleSaveCurrentLayout))

	// Monitor info
	c.router.HandleFunc("GET /api/monitors", c.requireAuth(c.handleListMonitors))
}

// requireAuth wraps a handler with authentication
func (c *Client) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c.config.AuthToken == "" {
			next(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if subtle.ConstantTimeCompare([]byte(token), []byte(c.config.AuthToken)) == 1 {
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
	var names []string
	for _, l := range allLayouts {
		names = append(names, l.Name)
	}

	common.WriteJSON(w, http.StatusOK, common.ListLayoutsResponse{
		Layouts:       names,
		CurrentLayout: c.currentLayout,
	})
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

	layout, ok := c.layouts.Get(req.Layout)
	if !ok {
		common.WriteError(w, http.StatusNotFound, fmt.Sprintf("layout %q not found", req.Layout))
		return
	}

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
	currentLayout, err := c.displayMgr.GetCurrentLayout(c.layouts)
	if err != nil {
		log.Printf("Failed to get current layout: %v", err)
	}

	// Fall back to cached value
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
		if m.Connected {
			monitorConfigs = append(monitorConfigs, common.Monitor{
				EDID:        m.EDID,
				Width:       m.Width,
				Height:      m.Height,
				RefreshRate: m.RefreshRate,
				PositionX:   m.PositionX,
				PositionY:   m.PositionY,
				Primary:     m.Primary,
				Enabled:     true,
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

// handleListMonitors returns connected monitor information
func (c *Client) handleListMonitors(w http.ResponseWriter, r *http.Request) {
	monitors, err := c.displayMgr.ListMonitors()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, monitors)
}

// Run starts the client
func Run(config *Config) error {
	client, err := New(config)
	if err != nil {
		return err
	}

	return client.Start()
}

// Start starts the HTTP server
func (c *Client) Start() error {
	c.server = &http.Server{
		Addr:         c.config.ListenAddr,
		Handler:      c.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Client starting on %s", c.config.ListenAddr)
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
