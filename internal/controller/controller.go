package controller

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
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/common"
	"github.com/trolleyman/ottoman/internal/config"
)

// Controller is the main orchestrator running on the Raspberry Pi
type Controller struct {
	config    *config.ControllerConfig
	router    *http.ServeMux
	server    *http.Server
	client    *http.Client
	startTime time.Time

	mu      sync.RWMutex
	secret  string
	localIP string
}

// Ensure Controller implements StrictServerInterface
var _ api.StrictServerInterface = (*Controller)(nil)

// getAgentAddr constructs the agent address from config
func (c *Controller) getAgentAddr() string {
	return fmt.Sprintf("%s:%d", c.config.Agent.IPAddress, c.config.Agent.Port)
}

// New creates a new controller instance
func New(config *config.ControllerConfig) (*Controller, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	c := &Controller{
		config: config,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		startTime: time.Now(),
		secret:    generateSecret(),
		localIP:   getOutboundIP(),
	}

	if err := c.setupRoutes(); err != nil {
		return nil, err
	}

	return c, nil
}

// setupRoutes configures HTTP routes
func (c *Controller) setupRoutes() error {
	// Create a wrapper mux that intercepts the trackpad endpoint
	innerMux := http.NewServeMux()

	// Use the generated strict handler
	strictHandler := api.NewStrictHandler(c, []api.StrictMiddlewareFunc{})
	api.HandlerWithOptions(strictHandler, api.StdHTTPServerOptions{
		BaseRouter: innerMux,
	})

	if err := common.SetupSPAHandler(innerMux); err != nil {
		return errors.Wrap(err, "")
	}

	// Create outer mux that intercepts trackpad and delegates rest to inner
	c.router = http.NewServeMux()
	c.router.HandleFunc("GET /api/trackpad", c.handleTrackpadWebSocket)
	c.router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Skip trackpad endpoint, delegate everything else to inner mux
		if r.Method == "GET" && r.URL.Path == "/api/trackpad" {
			http.NotFound(w, r)
			return
		}
		innerMux.ServeHTTP(w, r)
	})

	return nil
}

// CheckHealth implements api.StrictServerInterface
func (c *Controller) CheckHealth(ctx context.Context, request api.CheckHealthRequestObject) (api.CheckHealthResponseObject, error) {
	return api.CheckHealth200TextResponse("OK"), nil
}

// GetStatus implements api.StrictServerInterface
func (c *Controller) GetStatus(ctx context.Context, request api.GetStatusRequestObject) (api.GetStatusResponseObject, error) {
	_, port, _ := net.SplitHostPort(c.config.ListenAddress)
	if port == "" {
		port = "80"
	}

	uptime := time.Since(c.startTime).Round(time.Second).String()

	// IpAddress can be either a string or array - using string for simplicity
	var ipAddr api.StatusResponse_IpAddress
	if err := ipAddr.FromStatusResponseIpAddress0(c.localIP); err != nil {
		return nil, err
	}

	return api.GetStatus200JSONResponse{
		Status:    "ok",
		Version:   "dev",
		Uptime:    uptime,
		Hostname:  "",
		IpAddress: ipAddr,
		Port:      port,
		Secret:    c.secret,
	}, nil
}

// GetAgentStatus implements api.StrictServerInterface
func (c *Controller) GetAgentStatus(ctx context.Context, request api.GetAgentStatusRequestObject) (api.GetAgentStatusResponseObject, error) {
	url := fmt.Sprintf("http://%s/api/status/agent", c.getAgentAddr())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return api.GetAgentStatus502JSONResponse{
			Code:  http.StatusBadGateway,
			Error: "failed to create request",
		}, nil
	}

	if c.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.AuthToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return api.GetAgentStatus502JSONResponse{
			Code:  http.StatusBadGateway,
			Error: err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	var statusResp api.StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return api.GetAgentStatus502JSONResponse{
			Code:  http.StatusBadGateway,
			Error: "failed to decode response",
		}, nil
	}

	return api.GetAgentStatus200JSONResponse(statusResp), nil
}

// Auth implements api.StrictServerInterface
func (c *Controller) Auth(ctx context.Context, request api.AuthRequestObject) (api.AuthResponseObject, error) {
	if request.Body == nil || request.Body.Token == "" {
		msg := "missing token"
		return api.Auth401JSONResponse{
			Success: false,
			Message: &msg,
		}, nil
	}

	if subtle.ConstantTimeCompare([]byte(request.Body.Token), []byte(c.config.AuthToken)) != 1 {
		msg := "invalid token"
		return api.Auth401JSONResponse{
			Success: false,
			Message: &msg,
		}, nil
	}

	// Note: Cookie setting would need to be handled by middleware in strict mode
	// For now, just return success
	return api.Auth200JSONResponse{
		Success: true,
	}, nil
}

// Logout implements api.StrictServerInterface
func (c *Controller) Logout(ctx context.Context, request api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	// Note: Cookie clearing would need to be handled by middleware in strict mode
	return api.Logout200JSONResponse{
		Success: true,
	}, nil
}

// CheckAuth implements api.StrictServerInterface
func (c *Controller) CheckAuth(ctx context.Context, request api.CheckAuthRequestObject) (api.CheckAuthResponseObject, error) {
	authenticated := true
	return api.CheckAuth200JSONResponse{
		Authenticated: &authenticated,
	}, nil
}

// proxyRequest is a generic helper for proxying requests to the agent
func proxyRequest[T any](ctx context.Context, c *Controller, method, path string, body []byte, handler func(*http.Response) (T, error)) (T, error) {
	var zero T
	url := fmt.Sprintf("http://%s%s", c.getAgentAddr(), path)

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return zero, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.AuthToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	return handler(resp)
}

// Wake implements api.StrictServerInterface
func (c *Controller) Wake(ctx context.Context, request api.WakeRequestObject) (api.WakeResponseObject, error) {
	c.mu.RLock()
	macAddr := c.config.Agent.MACAddress
	c.mu.RUnlock()

	if macAddr == "" {
		msg := "no wake target configured"
		return api.Wake404JSONResponse{
			Code:  http.StatusNotFound,
			Error: msg,
		}, nil
	}

	// Send magic packet
	if err := SendToAllInterfaces(macAddr); err != nil {
		return api.Wake500JSONResponse{
			Code:  http.StatusInternalServerError,
			Error: err.Error(),
		}, nil
	}

	msg := fmt.Sprintf("Wake-on-LAN packet sent to %s", macAddr)
	return api.Wake200JSONResponse{
		Success: true,
		Message: &msg,
	}, nil
}

// GetLayouts implements api.StrictServerInterface
func (c *Controller) GetLayouts(ctx context.Context, request api.GetLayoutsRequestObject) (api.GetLayoutsResponseObject, error) {
	return proxyRequest[api.GetLayoutsResponseObject](ctx, c, "GET", "/api/layouts", nil, func(resp *http.Response) (api.GetLayoutsResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.LayoutsResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.GetLayouts200JSONResponse(result), nil
		case http.StatusUnauthorized:
			return api.GetLayouts401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		default:
			return api.GetLayouts502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// SwitchLayout implements api.StrictServerInterface
func (c *Controller) SwitchLayout(ctx context.Context, request api.SwitchLayoutRequestObject) (api.SwitchLayoutResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest[api.SwitchLayoutResponseObject](ctx, c, "POST", "/api/layouts/switch", body, func(resp *http.Response) (api.SwitchLayoutResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.SwitchLayoutResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.SwitchLayout200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.SwitchLayout400JSONResponse{Code: resp.StatusCode, Error: "Bad Request"}, nil
		case http.StatusUnauthorized:
			return api.SwitchLayout401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusNotFound:
			return api.SwitchLayout404JSONResponse{Code: resp.StatusCode, Error: "Layout not found"}, nil
		case http.StatusInternalServerError:
			return api.SwitchLayout500JSONResponse{Code: resp.StatusCode, Error: "Internal Server Error"}, nil
		default:
			return api.SwitchLayout502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// GetMonitors implements api.StrictServerInterface
func (c *Controller) GetMonitors(ctx context.Context, request api.GetMonitorsRequestObject) (api.GetMonitorsResponseObject, error) {
	return proxyRequest[api.GetMonitorsResponseObject](ctx, c, "GET", "/api/monitors", nil, func(resp *http.Response) (api.GetMonitorsResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.MonitorsResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.GetMonitors200JSONResponse(result), nil
		case http.StatusUnauthorized:
			return api.GetMonitors401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		default:
			return api.GetMonitors502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// GetCurrentLayout implements api.StrictServerInterface
func (c *Controller) GetCurrentLayout(ctx context.Context, request api.GetCurrentLayoutRequestObject) (api.GetCurrentLayoutResponseObject, error) {
	return proxyRequest[api.GetCurrentLayoutResponseObject](ctx, c, "GET", "/api/layouts/current", nil, func(resp *http.Response) (api.GetCurrentLayoutResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.SwitchLayoutResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.GetCurrentLayout200JSONResponse(result), nil
		case http.StatusUnauthorized:
			return api.GetCurrentLayout401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		default:
			return api.GetCurrentLayout502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// SaveCurrentLayout implements api.StrictServerInterface
func (c *Controller) SaveCurrentLayout(ctx context.Context, request api.SaveCurrentLayoutRequestObject) (api.SaveCurrentLayoutResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest[api.SaveCurrentLayoutResponseObject](ctx, c, "POST", "/api/layouts/save-current", body, func(resp *http.Response) (api.SaveCurrentLayoutResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.SaveLayoutResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.SaveCurrentLayout200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.SaveCurrentLayout400JSONResponse{Code: resp.StatusCode, Error: "Bad Request"}, nil
		case http.StatusUnauthorized:
			return api.SaveCurrentLayout401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SaveCurrentLayout500JSONResponse{Code: resp.StatusCode, Error: "Internal Server Error"}, nil
		default:
			return api.SaveCurrentLayout502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// RemoveLayout implements api.StrictServerInterface
func (c *Controller) RemoveLayout(ctx context.Context, request api.RemoveLayoutRequestObject) (api.RemoveLayoutResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest[api.RemoveLayoutResponseObject](ctx, c, "POST", "/api/layouts/remove", body, func(resp *http.Response) (api.RemoveLayoutResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.RemoveLayoutResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.RemoveLayout200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.RemoveLayout400JSONResponse{Code: resp.StatusCode, Error: "Bad Request"}, nil
		case http.StatusUnauthorized:
			return api.RemoveLayout401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusNotFound:
			return api.RemoveLayout404JSONResponse{Code: resp.StatusCode, Error: "Layout not found"}, nil
		case http.StatusInternalServerError:
			return api.RemoveLayout500JSONResponse{Code: resp.StatusCode, Error: "Internal Server Error"}, nil
		default:
			return api.RemoveLayout502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// Shutdown implements api.StrictServerInterface
func (c *Controller) Shutdown(ctx context.Context, request api.ShutdownRequestObject) (api.ShutdownResponseObject, error) {
	return proxyRequest[api.ShutdownResponseObject](ctx, c, "POST", "/api/shutdown", nil, func(resp *http.Response) (api.ShutdownResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.ShutdownResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.Shutdown200JSONResponse(result), nil
		case http.StatusUnauthorized:
			return api.Shutdown401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		default:
			return api.Shutdown502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// SimReset implements api.StrictServerInterface (stub)
func (c *Controller) SimReset(ctx context.Context, request api.SimResetRequestObject) (api.SimResetResponseObject, error) {
	return api.SimReset404JSONResponse{
		Code:  http.StatusNotFound,
		Error: "Not Found (Server not in simulation mode)",
	}, nil
}

// SimSetState implements api.StrictServerInterface (stub)
func (c *Controller) SimSetState(ctx context.Context, request api.SimSetStateRequestObject) (api.SimSetStateResponseObject, error) {
	return api.SimSetState404JSONResponse{
		Code:  http.StatusNotFound,
		Error: "Not Found (Server not in simulation mode)",
	}, nil
}

// SimState implements api.StrictServerInterface (stub)
func (c *Controller) SimState(ctx context.Context, request api.SimStateRequestObject) (api.SimStateResponseObject, error) {
	return api.SimState404JSONResponse{
		Code:  http.StatusNotFound,
		Error: "Not Found (Server not in simulation mode)",
	}, nil
}

// handleTrackpadWebSocket handles WebSocket connections for the trackpad
func (c *Controller) handleTrackpadWebSocket(w http.ResponseWriter, r *http.Request) {
	// Accept browser WebSocket
	browserConn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Printf("Trackpad proxy: accept error: %v", err)
		return
	}
	defer browserConn.CloseNow()

	// Dial client WebSocket
	clientURL := fmt.Sprintf("ws://%s/api/trackpad", c.getAgentAddr())
	dialOpts := &websocket.DialOptions{}
	if c.config.AuthToken != "" {
		dialOpts.HTTPHeader = http.Header{
			"Authorization": []string{"Bearer " + c.config.AuthToken},
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

// ConnectTrackpad implements api.StrictServerInterface (stub, actual handler registered separately)
func (c *Controller) ConnectTrackpad(ctx context.Context, request api.ConnectTrackpadRequestObject) (api.ConnectTrackpadResponseObject, error) {
	// This should never be called since we register the handler directly
	// But we need it to satisfy the StrictServerInterface
	return nil, fmt.Errorf("WebSocket handler should be called directly")
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

// Run starts the controller
func Run(config *config.ControllerConfig) error {
	controller, err := New(config)
	if err != nil {
		return err
	}

	return controller.Start()
}

// Start starts the HTTP server and background tasks
func (c *Controller) Start() error {
	c.server = &http.Server{
		Addr:         c.config.ListenAddress,
		Handler:      common.LoggingMiddleware(c.router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Controller starting on %s", c.config.ListenAddress)
		if err := c.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down controller...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return c.server.Shutdown(ctx)
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
