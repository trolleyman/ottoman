package client

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/common"
	"github.com/trolleyman/ottoman/internal/display"
)

// Client is the display control agent running on the desktop
type Client struct {
	config       *Config
	router       *http.ServeMux
	server       *http.Server
	layoutStore  *display.LayoutStore
	displayMgr   display.Manager
	startTime    time.Time
	currentLayout string
}

// New creates a new client instance
func New(config *Config) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	// Load layouts
	store, err := display.NewLayoutStore(config.LayoutsFile)
	if err != nil {
		log.Printf("Warning: failed to load layouts: %v", err)
		// Create empty store
		store = &display.LayoutStore{}
	}

	// Create display manager
	mgr, err := display.NewManager(store)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create display manager")
	}

	c := &Client{
		config:      config,
		layoutStore: store,
		displayMgr:  mgr,
		startTime:   time.Now(),
	}

	c.setupRoutes()

	return c, nil
}

// setupRoutes configures HTTP routes
func (c *Client) setupRoutes() {
	c.router = http.NewServeMux()

	// Health check (no auth)
	c.router.HandleFunc("GET /health", c.handleHealth)
	c.router.HandleFunc("GET /api/status", c.handleStatus)

	// Display control
	c.router.HandleFunc("GET /api/layouts", c.requireAuth(c.handleListLayouts))
	c.router.HandleFunc("POST /api/layouts/switch", c.requireAuth(c.handleSwitchLayout))
	c.router.HandleFunc("GET /api/layouts/current", c.requireAuth(c.handleCurrentLayout))

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
	layouts := c.layoutStore.List()
	currentLayout, _ := c.displayMgr.GetCurrentLayout()

	common.WriteJSON(w, http.StatusOK, common.ListLayoutsResponse{
		Layouts:       layouts,
		CurrentLayout: currentLayout,
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

	if err := c.displayMgr.ApplyLayout(req.Layout); err != nil {
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
	currentLayout, err := c.displayMgr.GetCurrentLayout()
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
