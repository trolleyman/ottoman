package controller

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

const (
	// agentHealthTTL bounds how often /api/status re-probes the agent's /health
	// when deciding whether to advertise the direct-agent endpoint.
	agentHealthTTL = 15 * time.Second
	// publicIPTTL bounds how often the network's public IP is re-fetched; home
	// IPs change rarely.
	publicIPTTL = 10 * time.Minute
	// publicIPRetry is how soon to retry after a failed public-IP fetch.
	publicIPRetry = time.Minute
)

// endpointState caches the two slow inputs to the /api/status endpoint
// hierarchy: whether the agent currently answers /health, and the network's
// public IP (for recognising tunnelled requests that originate from this same
// network). Both are refreshed in the background so /api/status never blocks
// on a probe.
type endpointState struct {
	mu            sync.Mutex
	refreshing    bool
	agentOK       bool
	agentCheck    time.Time // next agent /health probe due
	publicIP      string
	publicIPCheck time.Time // next public-IP fetch due
}

// refreshEndpointState returns the cached agent-health and public-IP values,
// kicking off a background refresh for whichever is stale. Callers get the
// current (possibly zero) values immediately; the SPA re-evaluates on its next
// status poll.
func (c *Controller) refreshEndpointState() (agentOK bool, publicIP string) {
	s := &c.endpoints
	now := time.Now()
	s.mu.Lock()
	agentOK, publicIP = s.agentOK, s.publicIP
	agentDue, ipDue := now.After(s.agentCheck), now.After(s.publicIPCheck)
	if s.refreshing || (!agentDue && !ipDue) {
		s.mu.Unlock()
		return agentOK, publicIP
	}
	s.refreshing = true
	s.mu.Unlock()

	go func() {
		var freshAgentOK bool
		if agentDue {
			freshAgentOK = c.agentHealthy()
		}
		var ip string
		var ipErr error
		if ipDue {
			ip, ipErr = fetchPublicIP()
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		s.refreshing = false
		if agentDue {
			s.agentOK = freshAgentOK
			s.agentCheck = time.Now().Add(agentHealthTTL)
		}
		if ipDue {
			if ipErr == nil {
				s.publicIP = ip
				s.publicIPCheck = time.Now().Add(publicIPTTL)
			} else {
				// Keep the last-known IP and retry soon.
				s.publicIPCheck = time.Now().Add(publicIPRetry)
			}
		}
	}()
	return agentOK, publicIP
}

// fetchPublicIP asks checkip.amazonaws.com (a bare-text "what is my IP"
// service) for this network's public IP.
func fetchPublicIP() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://checkip.amazonaws.com/", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("checkip returned status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(b))
	if net.ParseIP(ip) == nil {
		return "", errors.Errorf("checkip returned %q, not an IP", ip)
	}
	return ip, nil
}

// clientInfoKey carries the originating request's addressing info through to
// the strict handlers, which only receive a context.
type clientInfoKey struct{}

type clientInfo struct {
	remoteAddr   string // immediate peer (ip:port)
	forwardedFor string // X-Forwarded-For, set by tunnels/proxies (e.g. ngrok)
}

// withClientInfo stashes the request's peer address and X-Forwarded-For header
// in the context for clientIsLocal.
func withClientInfo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ci := clientInfo{remoteAddr: r.RemoteAddr, forwardedFor: r.Header.Get("X-Forwarded-For")}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientInfoKey{}, ci)))
	})
}

// effectiveClientIP resolves the originating client IP: the first
// X-Forwarded-For entry when a tunnel/proxy set one, otherwise the immediate
// peer address.
func effectiveClientIP(ci clientInfo) (netip.Addr, bool) {
	host := ci.remoteAddr
	if ci.forwardedFor != "" {
		host = strings.TrimSpace(strings.Split(ci.forwardedFor, ",")[0])
	} else if h, _, err := net.SplitHostPort(ci.remoteAddr); err == nil {
		host = h
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

// clientIsLocal reports whether the request's client appears to be on this
// network: a direct LAN/loopback peer, or a tunnelled request whose forwarded
// client IP equals the network's public IP (a client at home reaching us via
// ngrok egresses through the same public IP we do). The SPA uses this to
// decide whether redirecting to a LAN endpoint is safe when it cannot probe
// one itself (mixed-content rules on https pages).
func clientIsLocal(ctx context.Context, publicIP string) bool {
	ci, ok := ctx.Value(clientInfoKey{}).(clientInfo)
	if !ok {
		return false
	}
	a, ok := effectiveClientIP(ci)
	if !ok {
		return false
	}
	if a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return true
	}
	return publicIP != "" && a.String() == publicIP
}
