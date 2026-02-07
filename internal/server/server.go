package server

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/common"
)

// Server is the main orchestrator running on the Raspberry Pi
type Server struct {
	config    *Config
	router    *http.ServeMux
	server    *http.Server
	client    *http.Client
	startTime time.Time

	mu          sync.RWMutex
	wakeTargets map[string]WakeTarget
}

// New creates a new server instance
func New(config *Config) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	s := &Server{
		config: config,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		wakeTargets: make(map[string]WakeTarget),
		startTime:   time.Now(),
	}

	// Index wake targets by name
	for _, target := range config.WakeTargets {
		s.wakeTargets[strings.ToLower(target.Name)] = target
	}

	if err := s.setupRoutes(); err != nil {
		return nil, err
	}

	return s, nil
}

// setupRoutes configures HTTP routes
func (s *Server) setupRoutes() error {
	s.router = http.NewServeMux()

	// Health check (no auth required)
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("GET /api/status", s.handleStatus)

	// Auth endpoints
	s.router.HandleFunc("POST /api/auth", s.handleAuth)
	s.router.HandleFunc("POST /api/auth/logout", s.handleLogout)
	s.router.HandleFunc("GET /api/auth/check", s.requireAuth(s.handleAuthCheck))

	// Wake-on-LAN
	s.router.HandleFunc("POST /api/wake", s.requireAuth(s.handleWake))
	s.router.HandleFunc("GET /api/wake/targets", s.requireAuth(s.handleListWakeTargets))

	// Display control (proxied to client)
	s.router.HandleFunc("GET /api/layouts", s.requireAuth(s.handleListLayouts))
	s.router.HandleFunc("POST /api/layouts/switch", s.requireAuth(s.handleSwitchLayout))
	s.router.HandleFunc("GET /api/layouts/current", s.requireAuth(s.handleCurrentLayout))
	s.router.HandleFunc("POST /api/layouts/save-current", s.requireAuth(s.handleSaveCurrentLayout))
	s.router.HandleFunc("POST /api/layouts/remove", s.requireAuth(s.handleRemoveLayout))
	s.router.HandleFunc("GET /api/monitors", s.requireAuth(s.handleListMonitors))

	// Client status
	s.router.HandleFunc("GET /api/client/status", s.requireAuth(s.handleClientStatus))

	if err := common.SetupSPAHandler(s.router); err != nil {
		return errors.Wrap(err, "")
	}
	return nil
}

// requireAuth wraps a handler with authentication
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")

		// Token-based auth
		if s.config.AuthToken != "" {
			if token, ok := strings.CutPrefix(auth, "Bearer "); ok {
				if subtle.ConstantTimeCompare([]byte(token), []byte(s.config.AuthToken)) == 1 {
					next(w, r)
					return
				}
			}
		}

		// Check ottoman_token cookie
		if cookie, err := r.Cookie("ottoman_token"); err == nil {
			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.config.AuthToken)) == 1 {
				next(w, r)
				return
			}
		}

		// Basic auth
		if s.config.Username != "" {
			username, password, ok := r.BasicAuth()
			if ok && username == s.config.Username {
				// In production, compare against hashed password
				// For now, we just check if password hash matches
				if subtle.ConstantTimeCompare([]byte(password), []byte(s.config.PasswordHash)) == 1 {
					next(w, r)
					return
				}
			}
		}

		w.Header().Set("WWW-Authenticate", `Bearer realm="ottoman"`)
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
	}
}

// handleHealth returns a simple health check
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleStatus returns detailed status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startTime).Round(time.Second).String()
	common.WriteJSON(w, http.StatusOK, common.StatusResponse{
		Status:  "ok",
		Version: "dev", // Set at build time
		Uptime:  uptime,
	})
}

// handleAuth validates a token and sets an auth cookie
func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	var req common.AuthRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if subtle.ConstantTimeCompare([]byte(req.Token), []byte(s.config.AuthToken)) != 1 {
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
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
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
func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	common.WriteJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

// handleWake sends a wake-on-LAN packet
func (s *Server) handleWake(w http.ResponseWriter, r *http.Request) {
	var req common.WakeRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Find target
	s.mu.RLock()
	target, ok := s.wakeTargets[strings.ToLower(req.Target)]
	s.mu.RUnlock()

	var macAddr string
	if ok {
		macAddr = target.MACAddress
	} else {
		// Assume it's a MAC address directly
		macAddr = req.Target
	}

	// Send magic packet
	if err := SendToAllInterfaces(macAddr); err != nil {
		common.WriteJSON(w, http.StatusInternalServerError, common.WakeResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	common.WriteJSON(w, http.StatusOK, common.WakeResponse{
		Success: true,
		Message: fmt.Sprintf("Wake-on-LAN packet sent to %s", macAddr),
	})
}

// handleListWakeTargets returns available wake targets
func (s *Server) handleListWakeTargets(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	targets := make([]WakeTarget, 0, len(s.wakeTargets))
	for _, target := range s.wakeTargets {
		targets = append(targets, target)
	}

	common.WriteJSON(w, http.StatusOK, targets)
}

// handleListLayouts proxies to client to get available layouts
func (s *Server) handleListLayouts(w http.ResponseWriter, r *http.Request) {
	resp, err := s.proxyToClient("GET", "/api/layouts", nil)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleSwitchLayout proxies layout switch request to client
func (s *Server) handleSwitchLayout(w http.ResponseWriter, r *http.Request) {
	var req common.SwitchLayoutRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	body, _ := json.Marshal(req)
	resp, err := s.proxyToClient("POST", "/api/layouts/switch", body)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleListMonitors proxies to client to get connected monitors
func (s *Server) handleListMonitors(w http.ResponseWriter, r *http.Request) {
	resp, err := s.proxyToClient("GET", "/api/monitors", nil)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleCurrentLayout gets current layout from client
func (s *Server) handleCurrentLayout(w http.ResponseWriter, r *http.Request) {
	resp, err := s.proxyToClient("GET", "/api/layouts/current", nil)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleSaveCurrentLayout proxies save layout request to client
func (s *Server) handleSaveCurrentLayout(w http.ResponseWriter, r *http.Request) {
	var req common.SaveLayoutRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	body, _ := json.Marshal(req)
	resp, err := s.proxyToClient("POST", "/api/layouts/save-current", body)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleRemoveLayout proxies remove layout request to client
func (s *Server) handleRemoveLayout(w http.ResponseWriter, r *http.Request) {
	var req common.RemoveLayoutRequest
	if err := common.ReadJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	body, _ := json.Marshal(req)
	resp, err := s.proxyToClient("POST", "/api/layouts/remove", body)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleClientStatus checks if client is reachable
func (s *Server) handleClientStatus(w http.ResponseWriter, r *http.Request) {
	resp, err := s.proxyToClient("GET", "/health", nil)
	if err != nil {
		common.WriteJSON(w, http.StatusOK, common.StatusResponse{
			Status: "unreachable",
		})
		return
	}
	defer resp.Body.Close()

	common.WriteJSON(w, http.StatusOK, common.StatusResponse{
		Status: "ok",
	})
}

// proxyToClient sends a request to the client
func (s *Server) proxyToClient(method, path string, body []byte) (*http.Response, error) {
	url := fmt.Sprintf("http://%s%s", s.config.ClientAddr, path)

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Forward auth token if configured
	if s.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.config.AuthToken)
	}

	start := time.Now()
	resp, err := s.client.Do(req)
	duration := time.Since(start).Round(time.Microsecond)

	if err != nil {
		log.Printf("PROXY %s %s error: %v (%s)", method, path, err, duration)
		return nil, err
	}

	log.Printf("PROXY %s %s %d %s", method, path, resp.StatusCode, duration)
	return resp, nil
}

// Run starts the server
func Run(config *Config) error {
	server, err := New(config)
	if err != nil {
		return err
	}

	return server.Start()
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Start starts the HTTP server and background tasks
func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:         s.config.ListenAddr,
		Handler:      common.LoggingMiddleware(s.router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on %s", s.config.ListenAddr)
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// CheckStatus checks if a server is reachable
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
