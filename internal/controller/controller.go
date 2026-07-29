package controller

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/common"
	"github.com/trolleyman/ottoman/internal/config"
	"github.com/trolleyman/ottoman/internal/store"
	"github.com/trolleyman/ottoman/internal/tv"
)

// Controller is the main orchestrator running on the Raspberry Pi
type Controller struct {
	config    *config.ControllerConfig
	router    *http.ServeMux
	server    *http.Server
	client    *http.Client
	auth      *common.Authenticator
	agentBase *url.URL
	startTime time.Time

	mu      sync.RWMutex
	localIP string

	// TV mirror: a registry + pairing-key store synced from the agent while it's
	// up (see syncTVFromAgent), and a manager that drives the TV directly when
	// the agent is down (see the fallbacks in monitors.go). registry/tvStore are
	// nil only if their data dir couldn't be initialised.
	registry *store.Registry
	tvStore  *store.TVStore
	tv       *tv.Manager
	// lastTVExport is the raw body of the last TV export written to the mirror;
	// accessed only by the single sync goroutine, to skip unchanged rewrites.
	lastTVExport []byte

	// pending is a layout chosen while the desktop was off, applied when it
	// next answers (see pending.go). pendingPollInterval is how often the
	// orchestrator checks, while one is armed.
	pending             pendingLayout
	pendingPollInterval time.Duration
}

// Ensure Controller implements StrictServerInterface
var _ api.StrictServerInterface = (*Controller)(nil)

// agentURL builds an absolute URL for a path on the agent, carrying whatever
// scheme the config asked for. path must start with "/".
func (c *Controller) agentURL(path string) string {
	u := *c.agentBase
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	return u.String()
}

// agentWebSocketURL is agentURL for a WebSocket endpoint: https implies wss,
// http implies ws, so a TLS-fronted agent doesn't get dialled in the clear.
func (c *Controller) agentWebSocketURL(path string) string {
	u := *c.agentBase
	u.Scheme = "ws"
	if c.agentBase.Scheme == "https" {
		u.Scheme = "wss"
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	return u.String()
}

// New creates a new controller instance
func New(config *config.ControllerConfig) (*Controller, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	agentBase, err := config.Agent.BaseURL()
	if err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	c := &Controller{
		config: config,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		auth:      common.NewAuthenticator(config.AuthToken, config.LocalAuthRequired()),
		agentBase: agentBase,
		startTime: time.Now(),
		localIP:   getOutboundIP(),

		pendingPollInterval: defaultPendingPollInterval,
	}

	// TV mirror. These cache files hold a copy of the agent's TV registry +
	// pairing keys (distinct filenames so it's clearly a mirror, not the agent's
	// own store). A failure here shouldn't stop the controller from starting —
	// it just means no local TV fallback until the next successful sync.
	registry, err := store.NewRegistry(filepath.Join(store.DataDir(), "controller-tv-registry.json"))
	if err != nil {
		log.Printf("TV mirror unavailable (registry load failed): %v", err)
	} else {
		c.registry = registry
		c.tvStore = store.NewTVStore(filepath.Join(store.DataDir(), "controller-tv-keys.json"))
		c.tv = tv.NewManager(registry, c.tvStore)
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
	// Session endpoints, which need the ResponseWriter to set the cookie. On the
	// outer mux they shadow the generated handlers on the inner one.
	c.auth.RegisterAuthRoutes(c.router)
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
	}, nil
}

// GetAgentStatus implements api.StrictServerInterface
func (c *Controller) GetAgentStatus(ctx context.Context, request api.GetAgentStatusRequestObject) (api.GetAgentStatusResponseObject, error) {
	url := c.agentURL("/api/status/agent")
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
	url := c.agentURL(path)

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

// agentErrorMessage extracts the error message from a proxied agent error
// response so callers see the real failure (e.g. a TV or DDC error) instead of
// a generic status string. Falls back when the body carries no message.
func agentErrorMessage(resp *http.Response, fallback string) string {
	var e api.ErrorResponse
	if json.NewDecoder(resp.Body).Decode(&e) == nil && e.Error != "" {
		return e.Error
	}
	return fallback
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
	targets, err := SendToAllInterfaces(macAddr)
	if err != nil {
		return api.Wake500JSONResponse{
			Code:  http.StatusInternalServerError,
			Error: err.Error(),
		}, nil
	}

	labels := make([]string, len(targets))
	for i, t := range targets {
		labels[i] = t.String()
	}
	msg := fmt.Sprintf("Wake-on-LAN packet sent to %s via %s", macAddr, strings.Join(labels, ", "))

	// If the caller asked to wake into Windows, orchestrate it: once the Linux
	// agent is up, tell it to grub-reboot into Windows.
	if request.Body != nil && request.Body.Target != nil && *request.Body.Target == "windows" {
		go c.orchestrateWindowsBoot()
		msg += "; will boot into Windows once the agent is up"
	}

	// "Wake to the TV": the machine can't be told where to come up while it is
	// off, so hold the choice here and apply it on the way up.
	if request.Body != nil && request.Body.Layout != nil && *request.Body.Layout != "" {
		c.queueLayout(*request.Body.Layout)
		msg += fmt.Sprintf("; will switch to layout %q once the agent is up", *request.Body.Layout)
	}

	log.Printf("%s", msg)
	return api.Wake200JSONResponse{
		Success: true,
		Message: &msg,
	}, nil
}

// GetLayouts implements api.StrictServerInterface. When the agent is down it
// falls back to the mirrored layouts, so the list is still there when you most
// need it - choosing where the machine should come up before waking it.
func (c *Controller) GetLayouts(ctx context.Context, request api.GetLayoutsRequestObject) (api.GetLayoutsResponseObject, error) {
	resp, err := proxyRequest(ctx, c, "GET", "/api/layouts", nil, func(resp *http.Response) (api.GetLayoutsResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.LayoutsResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.GetLayouts200JSONResponse(c.withPending(result)), nil
		case http.StatusUnauthorized:
			return api.GetLayouts401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		default:
			return api.GetLayouts502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
	if err == nil {
		if _, unreachable := resp.(api.GetLayouts502JSONResponse); !unreachable {
			return resp, nil
		}
	}
	if mirror, ok := mirroredLayouts(); ok {
		return api.GetLayouts200JSONResponse(c.withPending(mirror)), nil
	}
	if err != nil {
		return api.GetLayouts502JSONResponse{Code: http.StatusBadGateway, Error: err.Error()}, nil
	}
	return resp, nil
}

// withPending attaches the queued layout, if any, so the UI can show that a
// choice is waiting for the desktop to come up.
func (c *Controller) withPending(l api.LayoutsResponse) api.LayoutsResponse {
	if id := c.PendingLayout(); id != "" {
		l.PendingLayout = &id
	}
	return l
}

// SwitchLayout implements api.StrictServerInterface. A switch aimed at a
// desktop that isn't up is queued rather than refused: picking "TV" from the
// phone while the machine is off (or still booting) is a statement of where it
// should come up, not a request that has failed.
func (c *Controller) SwitchLayout(ctx context.Context, request api.SwitchLayoutRequestObject) (api.SwitchLayoutResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	resp, err := c.switchLayoutOnAgent(ctx, body)
	if err == nil {
		if _, unreachable := resp.(api.SwitchLayout502JSONResponse); !unreachable {
			return resp, nil
		}
	}
	if request.Body == nil || request.Body.Layout == "" {
		return resp, err
	}
	c.queueLayout(request.Body.Layout)
	queued := true
	msg := "Desktop is offline - will switch when it comes up"
	return api.SwitchLayout200JSONResponse{
		Success:       true,
		CurrentLayout: "",
		Message:       &msg,
		Queued:        &queued,
	}, nil
}

func (c *Controller) switchLayoutOnAgent(ctx context.Context, body []byte) (api.SwitchLayoutResponseObject, error) {
	return proxyRequest(ctx, c, "POST", "/api/layouts/switch", body, func(resp *http.Response) (api.SwitchLayoutResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.SwitchLayoutResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.SwitchLayout200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.SwitchLayout400JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Bad Request")}, nil
		case http.StatusUnauthorized:
			return api.SwitchLayout401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusNotFound:
			return api.SwitchLayout404JSONResponse{Code: resp.StatusCode, Error: "Layout not found"}, nil
		case http.StatusInternalServerError:
			return api.SwitchLayout500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.SwitchLayout502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// GetMonitors implements api.StrictServerInterface. When the agent is down it
// falls back to the mirrored TV registry so the UI still shows (and can control)
// the TVs — the rest of the monitors are the desktop's and go with it.
func (c *Controller) GetMonitors(ctx context.Context, request api.GetMonitorsRequestObject) (api.GetMonitorsResponseObject, error) {
	resp, err := proxyRequest(ctx, c, "GET", "/api/monitors", nil, func(resp *http.Response) (api.GetMonitorsResponseObject, error) {
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
	if err != nil {
		if tvs := c.localTVMonitors(ctx); len(tvs) > 0 {
			return api.GetMonitors200JSONResponse(tvs), nil
		}
	}
	return resp, err
}

// GetCurrentLayout implements api.StrictServerInterface
func (c *Controller) GetCurrentLayout(ctx context.Context, request api.GetCurrentLayoutRequestObject) (api.GetCurrentLayoutResponseObject, error) {
	return proxyRequest(ctx, c, "GET", "/api/layouts/current", nil, func(resp *http.Response) (api.GetCurrentLayoutResponseObject, error) {
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
	return proxyRequest(ctx, c, "POST", "/api/layouts/save-current", body, func(resp *http.Response) (api.SaveCurrentLayoutResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.SaveLayoutResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.SaveCurrentLayout200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.SaveCurrentLayout400JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Bad Request")}, nil
		case http.StatusUnauthorized:
			return api.SaveCurrentLayout401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusInternalServerError:
			return api.SaveCurrentLayout500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.SaveCurrentLayout502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// RemoveLayout implements api.StrictServerInterface
func (c *Controller) RemoveLayout(ctx context.Context, request api.RemoveLayoutRequestObject) (api.RemoveLayoutResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/layouts/remove", body, func(resp *http.Response) (api.RemoveLayoutResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.RemoveLayoutResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.RemoveLayout200JSONResponse(result), nil
		case http.StatusBadRequest:
			return api.RemoveLayout400JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Bad Request")}, nil
		case http.StatusUnauthorized:
			return api.RemoveLayout401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusNotFound:
			return api.RemoveLayout404JSONResponse{Code: resp.StatusCode, Error: "Layout not found"}, nil
		case http.StatusInternalServerError:
			return api.RemoveLayout500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.RemoveLayout502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// UpdateLayout implements api.StrictServerInterface
func (c *Controller) UpdateLayout(ctx context.Context, request api.UpdateLayoutRequestObject) (api.UpdateLayoutResponseObject, error) {
	body, _ := json.Marshal(request.Body)
	return proxyRequest(ctx, c, "POST", "/api/layouts/update", body, func(resp *http.Response) (api.UpdateLayoutResponseObject, error) {
		switch resp.StatusCode {
		case http.StatusOK:
			var result api.UpdateLayoutResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, err
			}
			return api.UpdateLayout200JSONResponse(result), nil
		case http.StatusBadRequest:
			// Preserve the agent's message (e.g. an alias conflict) so the UI can
			// tell the user why the update was rejected.
			return api.UpdateLayout400JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Bad Request")}, nil
		case http.StatusUnauthorized:
			return api.UpdateLayout401JSONResponse{Code: resp.StatusCode, Error: "Unauthorized"}, nil
		case http.StatusNotFound:
			return api.UpdateLayout404JSONResponse{Code: resp.StatusCode, Error: "Layout not found"}, nil
		case http.StatusInternalServerError:
			return api.UpdateLayout500JSONResponse{Code: resp.StatusCode, Error: agentErrorMessage(resp, "Internal Server Error")}, nil
		default:
			return api.UpdateLayout502JSONResponse{Code: resp.StatusCode, Error: "Bad Gateway"}, nil
		}
	})
}

// Shutdown implements api.StrictServerInterface
func (c *Controller) Shutdown(ctx context.Context, request api.ShutdownRequestObject) (api.ShutdownResponseObject, error) {
	return proxyRequest(ctx, c, "POST", "/api/shutdown", nil, func(resp *http.Response) (api.ShutdownResponseObject, error) {
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
	clientURL := c.agentWebSocketURL("/api/trackpad")
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
		Handler:      common.LoggingMiddleware(c.auth.Middleware(c.router)),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Warn about any Wake-on-LAN misconfiguration up front.
	for _, w := range ValidateWakeConfig(c.config.Agent.MACAddress) {
		log.Printf("WARNING: %s", w)
	}

	common.WarnIfFrontEndBypassesAuth("controller", c.config.ListenAddress, c.config.LocalAuthRequired())

	// Mirror the agent's TV registry + pairing keys so the TV stays controllable
	// once the desktop is off. Stops when the controller shuts down.
	syncCtx, cancelSync := context.WithCancel(context.Background())
	defer cancelSync()
	c.startTVSync(syncCtx)
	c.startLayoutsSync(syncCtx)

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Controller starting on %s", c.config.ListenAddress)
		ln, err := common.ListenWithRetry("tcp", c.config.ListenAddress)
		if err != nil {
			log.Fatalf("Server error: %v", err)
		}
		if err := c.server.Serve(ln); err != http.ErrServerClosed {
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
	resp, err := client.Get(common.HealthURL(addr))
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
