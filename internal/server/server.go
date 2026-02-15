package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
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

	mu         sync.RWMutex
	wakeTarget *WakeTarget
	secret     string
	localIP    string
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
		startTime: time.Now(),
		secret:    generateSecret(),
		localIP:   getOutboundIP(),
	}

	// Use the first wake target if available
	if len(config.WakeTargets) > 0 {
		s.wakeTarget = &config.WakeTargets[0]
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
	s.router.HandleFunc("GET /api/status/client", s.requireAuth(s.handleClientStatus))

	// Auth endpoints
	s.router.HandleFunc("POST /api/auth", s.handleAuth)
	s.router.HandleFunc("POST /api/auth/logout", s.handleLogout)
	s.router.HandleFunc("GET /api/auth/check", s.requireAuth(s.handleAuthCheck))

	// Wake-on-LAN
	s.router.HandleFunc("POST /api/wake", s.requireAuth(s.handleWake))

	// Display control (proxied to client)
	s.router.HandleFunc("GET /api/layouts", s.requireAuth(s.handleListLayouts))
	s.router.HandleFunc("POST /api/layouts/switch", s.requireAuth(s.handleSwitchLayout))
	s.router.HandleFunc("GET /api/layouts/current", s.requireAuth(s.handleCurrentLayout))
	s.router.HandleFunc("POST /api/layouts/save-current", s.requireAuth(s.handleSaveCurrentLayout))
	s.router.HandleFunc("POST /api/layouts/remove", s.requireAuth(s.handleRemoveLayout))
	s.router.HandleFunc("GET /api/monitors", s.requireAuth(s.handleListMonitors))

	// Shutdown (proxied to client)
	s.router.HandleFunc("POST /api/shutdown", s.requireAuth(s.handleShutdown))

	// Trackpad (WebSocket proxy to client)
	s.router.HandleFunc("GET /api/trackpad", s.requireAuth(s.handleTrackpadProxy))

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
	// CORS: allow cross-origin health checks for local network redirect detection
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleStatus returns server status (does not contact client)
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	_, port, _ := net.SplitHostPort(s.config.ListenAddr)
	if port == "" {
		port = "80"
	}

	uptime := time.Since(s.startTime).Round(time.Second).String()
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"version":  "dev",
		"uptime":   uptime,
		"local_ip": s.localIP,
		"port":     port,
		"secret":   s.secret,
	})
}

// handleClientStatus proxies to client's /api/status
func (s *Server) handleClientStatus(w http.ResponseWriter, r *http.Request) {
	resp, err := s.proxyToClient("GET", "/api/status", nil)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
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
	s.mu.RLock()
	target := s.wakeTarget
	s.mu.RUnlock()

	if target == nil {
		common.WriteError(w, http.StatusNotFound, "no wake target configured")
		return
	}

	macAddr := target.MACAddress

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

// handleShutdown proxies shutdown request to client
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	resp, err := s.proxyToClient("POST", "/api/shutdown", nil)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleTrackpadProxy proxies a WebSocket connection to the client's trackpad endpoint.
func (s *Server) handleTrackpadProxy(w http.ResponseWriter, r *http.Request) {
	// Accept browser WebSocket
	browserConn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Printf("Trackpad proxy: accept error: %v", err)
		return
	}
	defer browserConn.CloseNow()

	// Dial client WebSocket
	clientURL := fmt.Sprintf("ws://%s/api/trackpad", s.config.ClientAddr)
	dialOpts := &websocket.DialOptions{}
	if s.config.AuthToken != "" {
		dialOpts.HTTPHeader = http.Header{
			"Authorization": []string{"Bearer " + s.config.AuthToken},
		}
	}

	ctx := r.Context()
	clientConn, _, err := websocket.Dial(ctx, clientURL, dialOpts)
	if err != nil {
		log.Printf("Trackpad proxy: failed to connect to client: %v", err)
		browserConn.Close(websocket.StatusInternalError, "client unreachable")
		return
	}
	defer clientConn.CloseNow()

	log.Printf("Trackpad proxy: connected")

	// Bidirectional pipe
	errc := make(chan error, 2)
	go func() { errc <- pipeWebSocket(ctx, browserConn, clientConn) }()
	go func() { errc <- pipeWebSocket(ctx, clientConn, browserConn) }()

	err = <-errc
	log.Printf("Trackpad proxy: closed: %v", err)
}

// pipeWebSocket copies messages from src to dst until an error occurs.
func pipeWebSocket(ctx context.Context, src, dst *websocket.Conn) error {
	for {
		msgType, data, err := src.Read(ctx)
		if err != nil {
			return err
		}
		if err := dst.Write(ctx, msgType, data); err != nil {
			return err
		}
	}
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

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func generateSecret() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
