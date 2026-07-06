package controller

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

const (
	// agentHealthTTL bounds how often /api/status re-probes the agent's /health
	// when deciding whether to advertise the direct-agent endpoint.
	agentHealthTTL = 15 * time.Second
	// publicIPTTL bounds how often the network's public IPs are re-fetched;
	// home IPs change rarely.
	publicIPTTL = 10 * time.Minute
	// publicIPRetry is how soon to retry after a failed public-IP fetch.
	publicIPRetry = time.Minute
)

// endpointState caches the two slow inputs to the /api/status endpoint
// hierarchy: whether the agent currently answers /health, and the network's
// public IPs (for recognising tunnelled requests that originate from this same
// network). Both are refreshed in the background so /api/status never blocks
// on a probe.
type endpointState struct {
	mu              sync.Mutex
	refreshing      bool
	agentOK         bool
	agentCheck      time.Time // next agent /health probe due
	publicIPs       []netip.Addr
	publicIPCheck   time.Time // next public-IP fetch due
	lastNonLocalXFF string    // last X-Forwarded-For logged as non-local
}

// refreshEndpointState returns the cached agent-health and public-IP values,
// kicking off a background refresh for whichever is stale. Callers get the
// current (possibly zero) values immediately; the SPA re-evaluates on its next
// status poll.
func (c *Controller) refreshEndpointState() (agentOK bool, publicIPs []netip.Addr) {
	s := &c.endpoints
	now := time.Now()
	s.mu.Lock()
	agentOK, publicIPs = s.agentOK, s.publicIPs
	agentDue, ipDue := now.After(s.agentCheck), now.After(s.publicIPCheck)
	if s.refreshing || (!agentDue && !ipDue) {
		s.mu.Unlock()
		return agentOK, publicIPs
	}
	s.refreshing = true
	s.mu.Unlock()

	go func() {
		var freshAgentOK bool
		if agentDue {
			freshAgentOK = c.agentHealthy()
		}
		var ips []netip.Addr
		var ipErr error
		if ipDue {
			ips, ipErr = fetchPublicIPs()
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
				if !slices.Equal(ips, s.publicIPs) {
					log.Printf("Network public IP(s): %v", ips)
				}
				s.publicIPs = ips
				s.publicIPCheck = time.Now().Add(publicIPTTL)
			} else {
				// Keep the last-known IPs and retry soon.
				log.Printf("Public IP fetch failed (retrying in %s): %v", publicIPRetry, ipErr)
				s.publicIPCheck = time.Now().Add(publicIPRetry)
			}
		}
	}()
	return agentOK, publicIPs
}

// fetchPublicIPs asks checkip.amazonaws.com (a bare-text "what is my IP"
// service) for this network's public IPv4 and IPv6 addresses, forcing each
// address family in turn. Either family may be absent; it's an error only if
// both are.
func fetchPublicIPs() ([]netip.Addr, error) {
	var ips []netip.Addr
	var firstErr error
	for _, network := range []string{"tcp4", "tcp6"} {
		ip, err := fetchPublicIP(network)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ips = append(ips, ip)
	}
	if len(ips) == 0 {
		return nil, firstErr
	}
	return ips, nil
}

func fetchPublicIP(network string) (netip.Addr, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, addr)
			},
		},
	}
	resp, err := client.Get("https://checkip.amazonaws.com/")
	if err != nil {
		return netip.Addr{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return netip.Addr{}, errors.Errorf("checkip returned status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return netip.Addr{}, err
	}
	a, err := netip.ParseAddr(strings.TrimSpace(string(b)))
	if err != nil {
		return netip.Addr{}, errors.Wrap(err, "checkip returned a non-IP")
	}
	return a.Unmap(), nil
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
// peer address. Accepts bare-IP, ip:port and [v6]:port forms.
func effectiveClientIP(ci clientInfo) (netip.Addr, bool) {
	host := ci.remoteAddr
	if ci.forwardedFor != "" {
		host = strings.TrimSpace(strings.Split(ci.forwardedFor, ",")[0])
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return a.Unmap(), true
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		if a, err := netip.ParseAddr(h); err == nil {
			return a.Unmap(), true
		}
	}
	return netip.Addr{}, false
}

// matchesPublicIP reports whether a client address belongs to this network:
// exact match for IPv4 (the whole household shares one NATed address), same
// /64 for IPv6 (every device has its own address within the delegated prefix,
// so equality would never hold).
func matchesPublicIP(a netip.Addr, publics []netip.Addr) bool {
	for _, p := range publics {
		if a == p {
			return true
		}
		if a.Is6() && p.Is6() {
			if prefix, err := p.Prefix(64); err == nil && prefix.Contains(a) {
				return true
			}
		}
	}
	return false
}

// clientIsLocal reports whether the request's client appears to be on this
// network: a direct LAN/loopback peer, or a tunnelled request whose forwarded
// client IP matches the network's public IPs (a client at home reaching us via
// ngrok egresses through the same connection we do). The SPA uses this to
// decide whether redirecting to a LAN endpoint is safe when it cannot probe
// one itself (mixed-content rules on https pages).
func (c *Controller) clientIsLocal(ctx context.Context, publicIPs []netip.Addr) bool {
	ci, ok := ctx.Value(clientInfoKey{}).(clientInfo)
	if !ok {
		return false
	}
	a, parsed := effectiveClientIP(ci)
	local := parsed &&
		(a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || matchesPublicIP(a, publicIPs))

	// Log tunnelled clients classified non-local (once per distinct header
	// value) — the key signal when debugging why a redirect didn't fire.
	if !local && ci.forwardedFor != "" {
		s := &c.endpoints
		s.mu.Lock()
		if s.lastNonLocalXFF != ci.forwardedFor {
			s.lastNonLocalXFF = ci.forwardedFor
			log.Printf("Client not local: X-Forwarded-For=%q remote=%s public=%v",
				ci.forwardedFor, ci.remoteAddr, publicIPs)
		}
		s.mu.Unlock()
	}
	return local
}
